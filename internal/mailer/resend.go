package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const defaultResendAPIBaseURL = "https://api.resend.com"

type Message struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	HTML    string `json:"html,omitempty"`
	Text    string `json:"text,omitempty"`
}

type resendMessage struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	HTML    string `json:"html,omitempty"`
	Text    string `json:"text,omitempty"`
}

type Client struct {
	APIKey     string
	From       string
	APIBaseURL string
	HTTPClient *http.Client
}

func FromEnv() (*Client, error) {
	apiKey := strings.TrimSpace(os.Getenv("RESEND_API_KEY"))
	from := strings.TrimSpace(os.Getenv("CODELOCAL_EMAIL_FROM"))
	if apiKey == "" {
		return nil, errors.New("RESEND_API_KEY is required")
	}
	if from == "" {
		return nil, errors.New("CODELOCAL_EMAIL_FROM is required")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("RESEND_API_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = defaultResendAPIBaseURL
	}
	return &Client{APIKey: apiKey, From: from, APIBaseURL: baseURL, HTTPClient: &http.Client{Timeout: 12 * time.Second}}, nil
}

func (c *Client) Send(ctx context.Context, message Message, idempotencyKey string) error {
	if strings.TrimSpace(message.To) == "" || strings.TrimSpace(message.Subject) == "" {
		return errors.New("email recipient and subject are required")
	}
	payload := resendMessage{From: c.From, To: message.To, Subject: message.Subject, HTML: message.HTML, Text: message.Text}
	return c.post(ctx, "/emails", payload, idempotencyKey)
}

func (c *Client) SendBatch(ctx context.Context, messages []Message, idempotencyKey string) error {
	if len(messages) == 0 {
		return nil
	}
	if len(messages) > 100 {
		return fmt.Errorf("resend batch supports at most 100 emails, got %d", len(messages))
	}
	payload := make([]resendMessage, 0, len(messages))
	for _, message := range messages {
		if strings.TrimSpace(message.To) == "" || strings.TrimSpace(message.Subject) == "" {
			return errors.New("email recipient and subject are required")
		}
		payload = append(payload, resendMessage{From: c.From, To: message.To, Subject: message.Subject, HTML: message.HTML, Text: message.Text})
	}
	return c.post(ctx, "/emails/batch", payload, idempotencyKey)
}

func (c *Client) post(ctx context.Context, path string, payload any, idempotencyKey string) error {
	if c == nil || strings.TrimSpace(c.APIKey) == "" || strings.TrimSpace(c.From) == "" {
		return errors.New("mailer is not configured")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	baseURL := strings.TrimRight(c.APIBaseURL, "/")
	if baseURL == "" {
		baseURL = defaultResendAPIBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	if key := strings.TrimSpace(idempotencyKey); key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 32<<10))
		return nil
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	message := strings.TrimSpace(string(raw))
	if message == "" {
		message = resp.Status
	}
	return fmt.Errorf("resend request failed (%d): %s", resp.StatusCode, message)
}
