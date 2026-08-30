package cloudserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/mcpgateway"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func validateChatGPTFileURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
		return nil, errors.New("ChatGPT file download URL must be a public HTTPS URL")
	}
	return parsed, nil
}

func publicDownloadIP(ip net.IP) bool {
	return ip != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()
}

func blogFileDownloadClient() *http.Client {
	dialer := &net.Dialer{Timeout: 8 * time.Second, KeepAlive: 20 * time.Second}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil || len(addresses) == 0 {
			return nil, errors.New("ChatGPT file host could not be resolved")
		}
		for _, candidate := range addresses {
			if !publicDownloadIP(candidate.IP) {
				return nil, errors.New("ChatGPT file host resolved to a non-public address")
			}
		}
		var lastErr error
		for _, candidate := range addresses {
			conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		return nil, lastErr
	}
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("too many ChatGPT file redirects")
			}
			_, err := validateChatGPTFileURL(req.URL.String())
			return err
		},
	}
}

func durableImportedImageType(data []byte) string {
	switch {
	case len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}):
		return "image/png"
	case len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
		return "image/jpeg"
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "image/webp"
	default:
		return ""
	}
}

func downloadChatGPTImage(ctx context.Context, ref mcpgateway.BlogFileRef, maxBytes int64) ([]byte, string, error) {
	if strings.TrimSpace(ref.FileID) == "" || len(ref.FileID) > 256 {
		return nil, "", errors.New("invalid ChatGPT file ID")
	}
	parsed, err := validateChatGPTFileURL(ref.DownloadURL)
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "image/png,image/jpeg,image/webp")
	response, err := blogFileDownloadClient().Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("download ChatGPT image: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("download ChatGPT image returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maxBytes {
		return nil, "", fmt.Errorf("ChatGPT image exceeds %d bytes", maxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("read ChatGPT image: %w", err)
	}
	if len(data) == 0 || int64(len(data)) > maxBytes {
		return nil, "", fmt.Errorf("ChatGPT image must be between 1 and %d bytes", maxBytes)
	}
	contentType := durableImportedImageType(data)
	if contentType == "" {
		return nil, "", errors.New("ChatGPT file is not a supported PNG, JPEG, or WebP image")
	}
	return data, contentType, nil
}

func (s *Server) ImportBlogImage(ctx context.Context, userID string, ref mcpgateway.BlogFileRef) (cloud.MediaAsset, error) {
	if s == nil || s.Store == nil || s.Media == nil {
		return cloud.MediaAsset{}, errors.New("blog media storage unavailable")
	}
	data, contentType, err := downloadChatGPTImage(ctx, ref, s.Media.maxBytes)
	if err != nil {
		return cloud.MediaAsset{}, err
	}
	digest := sha256.Sum256(data)
	hash := hex.EncodeToString(digest[:])
	asset, _, err := s.Store.EnsureMediaAsset(ctx, userID, hash, contentType, int64(len(data)), false)
	if err != nil {
		return cloud.MediaAsset{}, err
	}
	if asset.Status == "ready" {
		return asset, nil
	}
	if asset.Status == "failed" {
		asset, err = s.Store.ResetMediaAssetProcessing(ctx, userID, asset.ID, false)
		if err != nil {
			return cloud.MediaAsset{}, err
		}
	}
	sourceKey := durableMediaSourceKey(s.Media, userID, asset)
	_, err = s.Media.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:       aws.String(s.Media.bucket),
		Key:          aws.String(sourceKey),
		Body:         bytes.NewReader(data),
		ContentType:  aws.String(contentType),
		CacheControl: aws.String("private, no-store"),
		Metadata:     map[string]string{"content-sha256": hash, "source": "chatgpt-file"},
	})
	if err != nil {
		_ = s.Store.FailMediaAsset(ctx, userID, asset.ID, "source_upload_failed")
		return cloud.MediaAsset{}, fmt.Errorf("store ChatGPT media source: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://codelocal.invalid/internal-media-import", nil)
	if err != nil {
		return cloud.MediaAsset{}, err
	}
	completed, err := s.processMediaAsset(request, asset)
	if err != nil {
		_ = s.Store.FailMediaAsset(ctx, userID, asset.ID, mediaAssetErrorCode(err))
		return cloud.MediaAsset{}, err
	}
	return completed, nil
}
