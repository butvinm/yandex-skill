package tracker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/butvinm/yandex-cli/internal/auth"
)

type Client struct {
	http    *http.Client
	baseURL string
	headers http.Header
}

func New(cfg auth.Config) *Client {
	return &Client{
		http:    &http.Client{Timeout: 30 * time.Second},
		baseURL: strings.TrimRight(cfg.TrackerBaseURL, "/"),
		headers: cfg.TrackerHeaders(),
	}
}

type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("tracker api %d: %s", e.Status, e.Message)
}

// Do executes a request, decodes the response into out if non-nil and the
// response is JSON, and returns the *http.Response so callers can inspect
// pagination headers (Tracker uses Link rel=next).
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

// DoPaginated calls Do and follows Link rel=next headers, decoding each page
// into a slice via the appendFn callback. The first call uses the given path;
// subsequent calls use the absolute URL from the Link header.
func (c *Client) DoPaginated(ctx context.Context, path string, body any, fetchPage func(rawJSON []byte) error) error {
	url := path
	for {
		var raw json.RawMessage
		resp, err := c.Do(ctx, methodFor(body), url, body, &raw)
		if err != nil {
			return err
		}
		if err := fetchPage([]byte(raw)); err != nil {
			return err
		}
		next := nextPageURL(resp.Header.Get("Link"))
		if next == "" {
			return nil
		}
		url = next
		body = nil // body only on first request for POST search; subsequent next-links are GET-able
	}
}

func methodFor(body any) string {
	if body == nil {
		return http.MethodGet
	}
	return http.MethodPost
}

// nextPageURL parses a Link header and returns the URL with rel="next", or "".
// Format: <https://...>; rel="next", <https://...>; rel="prev"
func nextPageURL(link string) string {
	if link == "" {
		return ""
	}
	for _, part := range strings.Split(link, ",") {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, `rel="next"`) {
			continue
		}
		if i := strings.Index(part, "<"); i != -1 {
			if j := strings.Index(part[i+1:], ">"); j != -1 {
				return part[i+1 : i+1+j]
			}
		}
	}
	return ""
}

func extractErrorMsg(body []byte) string {
	var apiErr struct {
		ErrorMessages []string `json:"errorMessages"`
		Errors        map[string]string
	}
	if err := json.Unmarshal(body, &apiErr); err == nil {
		if len(apiErr.ErrorMessages) > 0 {
			return strings.Join(apiErr.ErrorMessages, "; ")
		}
		if len(apiErr.Errors) > 0 {
			parts := make([]string, 0, len(apiErr.Errors))
			for k, v := range apiErr.Errors {
				parts = append(parts, k+": "+v)
			}
			return strings.Join(parts, "; ")
		}
	}
	if s := strings.TrimSpace(string(body)); s != "" {
		return s
	}
	return "(no body)"
}
