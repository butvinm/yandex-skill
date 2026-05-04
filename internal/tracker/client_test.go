package tracker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/butvinm/yandex-skill/internal/auth"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	cfg := auth.Config{Token: "tok", OrgID: "org", TrackerBaseURL: srv.URL}
	return New(cfg), srv
}

func TestClient_Do_GET_Success(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("X-Cloud-Org-ID"); got != "org" {
			t.Errorf("X-Cloud-Org-ID = %q (default tenancy is Cloud)", got)
		}
		if r.URL.Path != "/v3/issues/FOO-1" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"key":"FOO-1","summary":"hi"}`)
	})

	var got struct {
		Key     string `json:"key"`
		Summary string `json:"summary"`
	}
	resp, err := c.Do(context.Background(), http.MethodGet, "/v3/issues/FOO-1", nil, &got)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
	if got.Key != "FOO-1" || got.Summary != "hi" {
		t.Errorf("decoded = %+v", got)
	}
}

func TestClient_Do_POST_BodyMarshaled(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["query"] != "Status: Open" {
			t.Errorf("body query = %q", body["query"])
		}
		_, _ = io.WriteString(w, `[]`)
	})

	var out []any
	_, err := c.Do(context.Background(), http.MethodPost, "/v3/issues/_search",
		map[string]string{"query": "Status: Open"}, &out)
	if err != nil {
		t.Fatal(err)
	}
}

func TestClient_Do_4xx_APIError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = io.WriteString(w, `{"errorMessages":["not found"]}`)
	})
	_, err := c.Do(context.Background(), http.MethodGet, "/v3/issues/X-1", nil, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err type = %T", err)
	}
	if apiErr.Status != 404 {
		t.Errorf("status = %d", apiErr.Status)
	}
	if apiErr.Message != "not found" {
		t.Errorf("message = %q", apiErr.Message)
	}
}

func TestClient_DoRaw_Success_BodyStreamableAndOpen(t *testing.T) {
	payload := []byte("hello-binary-bytes")
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(payload)
	})

	resp, err := c.DoRaw(context.Background(), http.MethodGet, "/v3/issues/FOO-1/attachments/1/x.bin", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("body = %q, want %q", got, payload)
	}
}

func TestClient_DoRaw_4xx_APIError_BodyClosed(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = io.WriteString(w, `{"errorMessages":["not found"]}`)
	})
	resp, err := c.DoRaw(context.Background(), http.MethodGet, "/v3/issues/FOO-1/attachments/1/x.bin", "", nil)
	if err == nil {
		t.Fatal("want error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err type = %T", err)
	}
	if apiErr.Status != 404 {
		t.Errorf("status = %d", apiErr.Status)
	}
	if apiErr.Message != "not found" {
		t.Errorf("message = %q", apiErr.Message)
	}
	if resp == nil {
		t.Fatal("resp nil even though error returned with response")
	}
}

func TestClient_DoPaginated_FollowsLink(t *testing.T) {
	pages := 0
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/v3/issues/_search", func(w http.ResponseWriter, r *http.Request) {
		pages++
		if pages == 1 {
			w.Header().Set("Link", `<`+srv.URL+`/v3/issues/_search?page=2>; rel="next"`)
			_, _ = io.WriteString(w, `[{"key":"A-1"}]`)
			return
		}
		// page 2: no Link header → loop exits
		_, _ = io.WriteString(w, `[{"key":"A-2"}]`)
	})

	cfg := auth.Config{Token: "tok", OrgID: "org", TrackerBaseURL: srv.URL}
	c := New(cfg)

	var keys []string
	err := c.DoPaginated(context.Background(), "/v3/issues/_search", map[string]string{"queue": "A"},
		func(raw []byte) error {
			var batch []struct{ Key string }
			if err := json.Unmarshal(raw, &batch); err != nil {
				return err
			}
			for _, x := range batch {
				keys = append(keys, x.Key)
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if pages != 2 {
		t.Errorf("expected 2 pages, got %d", pages)
	}
	if len(keys) != 2 || keys[0] != "A-1" || keys[1] != "A-2" {
		t.Errorf("keys = %v", keys)
	}
}

func TestNextPageURL(t *testing.T) {
	tests := []struct {
		link, want string
	}{
		{"", ""},
		{`<https://x/y?p=2>; rel="next"`, "https://x/y?p=2"},
		{`<https://x/y?p=2>; rel="next", <https://x/y?p=1>; rel="prev"`, "https://x/y?p=2"},
		{`<https://x/y?p=1>; rel="prev"`, ""},
	}
	for _, tt := range tests {
		if got := nextPageURL(tt.link); got != tt.want {
			t.Errorf("nextPageURL(%q) = %q, want %q", tt.link, got, tt.want)
		}
	}
}
