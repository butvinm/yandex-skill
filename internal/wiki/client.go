package wiki

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/butvinm/yandex-skill/internal/auth"
)

type Client struct {
	http    *http.Client
	baseURL string
	headers http.Header
}

func New(cfg auth.Config) *Client {
	return &Client{
		http:    &http.Client{Timeout: 30 * time.Second},
		baseURL: strings.TrimRight(cfg.WikiBaseURL, "/"),
		headers: cfg.WikiHeaders(),
	}
}

type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("wiki api %d: %s", e.Status, e.Message)
}

func (c *Client) Do(ctx context.Context, method, url string, body, out any) (*http.Response, error) {
	if !strings.HasPrefix(url, "http") {
		url = c.baseURL + url
	}
	var bodyReader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	for k, vs := range c.headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		buf, _ := io.ReadAll(resp.Body)
		return resp, &APIError{Status: resp.StatusCode, Message: extractErrorMsg(buf)}
	}
	if out != nil {
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp, fmt.Errorf("decode response: %w", err)
		}
	}
	return resp, nil
}

// DoRaw issues a request with a raw body and returns the live response.
// Use it for binary uploads (PUT octet-stream) and binary downloads (GET).
// On non-2xx the body is consumed and an *APIError is returned. On success
// the caller MUST close resp.Body.
func (c *Client) DoRaw(ctx context.Context, method, url, contentType string, body io.Reader) (*http.Response, error) {
	if !strings.HasPrefix(url, "http") {
		url = c.baseURL + url
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	for k, vs := range c.headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		buf, _ := io.ReadAll(resp.Body)
		return resp, &APIError{Status: resp.StatusCode, Message: extractErrorMsg(buf)}
	}
	return resp, nil
}

func extractErrorMsg(body []byte) string {
	var apiErr struct {
		Detail  string `json:"detail"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &apiErr); err == nil {
		if apiErr.Detail != "" {
			return apiErr.Detail
		}
		if apiErr.Message != "" {
			return apiErr.Message
		}
		if apiErr.Error != "" {
			return apiErr.Error
		}
	}
	if s := strings.TrimSpace(string(body)); s != "" {
		return s
	}
	return "(no body)"
}
