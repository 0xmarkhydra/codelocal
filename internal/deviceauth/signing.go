package deviceauth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	HeaderTimestamp = "X-CodeLocal-Signature-Timestamp"
	HeaderNonce     = "X-CodeLocal-Signature-Nonce"
	HeaderSignature = "X-CodeLocal-Signature"
	MaxClockSkew    = 90 * time.Second
	maxSignedBody   = 20 << 20
)

func GenerateKeyPair() (publicKey, privateKey string, err error) {
	publicRaw, privateRaw, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return base64.RawURLEncoding.EncodeToString(publicRaw), base64.RawURLEncoding.EncodeToString(privateRaw), nil
}

func ValidPublicKey(value string) bool {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	return err == nil && len(raw) == ed25519.PublicKeySize
}

func newNonce() (string, error) {
	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func canonical(method, requestURI string, body []byte, timestamp int64, nonce string) []byte {
	sum := sha256.Sum256(body)
	return []byte(strings.ToUpper(method) + "\n" + requestURI + "\n" + strconv.FormatInt(timestamp, 10) + "\n" + nonce + "\n" + hex.EncodeToString(sum[:]))
}

func requestURI(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	uri := u.RequestURI()
	if uri == "" {
		uri = "/"
	}
	return uri, nil
}

func SignatureHeaders(method, rawURL string, body []byte, privateKey string, now time.Time) (http.Header, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(privateKey))
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid device private key")
	}
	uri, err := requestURI(rawURL)
	if err != nil {
		return nil, err
	}
	nonce, err := newNonce()
	if err != nil {
		return nil, err
	}
	timestamp := now.Unix()
	signature := ed25519.Sign(ed25519.PrivateKey(raw), canonical(method, uri, body, timestamp, nonce))
	headers := make(http.Header)
	headers.Set(HeaderTimestamp, strconv.FormatInt(timestamp, 10))
	headers.Set(HeaderNonce, nonce)
	headers.Set(HeaderSignature, base64.RawURLEncoding.EncodeToString(signature))
	return headers, nil
}

func SignRequest(req *http.Request, body []byte, privateKey string, now time.Time) error {
	if strings.TrimSpace(privateKey) == "" {
		return nil
	}
	headers, err := SignatureHeaders(req.Method, req.URL.String(), body, privateKey, now)
	if err != nil {
		return err
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Set(key, value)
		}
	}
	return nil
}

func RequestNonce(req *http.Request) string {
	if req == nil {
		return ""
	}
	return strings.TrimSpace(req.Header.Get(HeaderNonce))
}

func VerifyRequest(req *http.Request, publicKey string, now time.Time) error {
	publicRaw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(publicKey))
	if err != nil || len(publicRaw) != ed25519.PublicKeySize {
		return errors.New("invalid device public key")
	}
	timestamp, err := strconv.ParseInt(strings.TrimSpace(req.Header.Get(HeaderTimestamp)), 10, 64)
	if err != nil {
		return errors.New("device signature timestamp missing")
	}
	at := time.Unix(timestamp, 0)
	if at.Before(now.Add(-MaxClockSkew)) || at.After(now.Add(MaxClockSkew)) {
		return errors.New("device signature timestamp outside allowed skew")
	}
	nonce := RequestNonce(req)
	nonceRaw, err := base64.RawURLEncoding.DecodeString(nonce)
	if err != nil || len(nonceRaw) < 16 || len(nonceRaw) > 64 {
		return errors.New("device signature nonce missing")
	}
	signature, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(req.Header.Get(HeaderSignature)))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return errors.New("device signature missing")
	}
	body := []byte(nil)
	if req.Body != nil {
		body, err = io.ReadAll(io.LimitReader(req.Body, maxSignedBody+1))
		if err != nil {
			return err
		}
		if len(body) > maxSignedBody {
			return errors.New("signed request body too large")
		}
		_ = req.Body.Close()
		req.Body = io.NopCloser(strings.NewReader(string(body)))
	}
	uri := req.URL.RequestURI()
	if uri == "" {
		uri = "/"
	}
	if !ed25519.Verify(ed25519.PublicKey(publicRaw), canonical(req.Method, uri, body, timestamp, nonce), signature) {
		return errors.New("invalid device signature")
	}
	return nil
}
