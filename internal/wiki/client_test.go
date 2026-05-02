package wiki

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/butvinm/yandex-skill/internal/auth"
)

func TestClient_Do_HeadersIncludeCloudOrgID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Cloud-Org-ID"); got != "org" {
			t.Errorf("X-Cloud-Org-ID = %q (default tenancy is Cloud)", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q", got)
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	c := New(auth.Config{Token: "tok", OrgID: "org", WikiBaseURL: srv.URL})
	_, err := c.Do(context.Background(), http.MethodGet, "/v1/pages", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestClient_Do_4xx_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = io.WriteString(w, `{"detail":"forbidden"}`)
	}))
	defer srv.Close()

	c := New(auth.Config{Token: "tok", OrgID: "org", WikiBaseURL: srv.URL})
	_, err := c.Do(context.Background(), http.MethodGet, "/v1/pages", nil, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 403 || apiErr.Message != "forbidden" {
		t.Errorf("err = %v", err)
	}
}

func TestClient_DoRaw_PutOctetStream(t *testing.T) {
	var seenMethod, seenPath, seenCT string
	var seenBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenMethod = r.Method
		seenPath = r.URL.Path
		seenCT = r.Header.Get("Content-Type")
		seenBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := New(auth.Config{Token: "tok", OrgID: "org", WikiBaseURL: srv.URL})
	resp, err := c.DoRaw(context.Background(), http.MethodPut, "/v1/upload_sessions/abc/upload_part?part_number=1", "application/octet-stream", strings.NewReader("payload"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if seenMethod != http.MethodPut {
		t.Errorf("method = %s", seenMethod)
	}
	if seenPath != "/v1/upload_sessions/abc/upload_part" {
		t.Errorf("path = %s", seenPath)
	}
	if seenCT != "application/octet-stream" {
		t.Errorf("content-type = %s", seenCT)
	}
	if string(seenBody) != "payload" {
		t.Errorf("body = %q", seenBody)
	}
}

func TestClient_DoRaw_4xx_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = io.WriteString(w, `{"error_code":"oops","debug_message":"boom"}`)
	}))
	defer srv.Close()

	c := New(auth.Config{Token: "tok", OrgID: "org", WikiBaseURL: srv.URL})
	_, err := c.DoRaw(context.Background(), http.MethodGet, "/v1/x", "", nil)
	if err == nil {
		t.Fatal("want error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 500 {
		t.Errorf("err = %v", err)
	}
}

func TestExtractErrorMsg(t *testing.T) {
	cases := map[string]string{
		`{"detail":"x"}`:  "x",
		`{"message":"y"}`: "y",
		`{"error":"z"}`:   "z",
		`plain text`:      "plain text",
		``:                "(no body)",
	}
	for in, want := range cases {
		if got := extractErrorMsg([]byte(in)); got != want {
			t.Errorf("extractErrorMsg(%q) = %q, want %q", in, got, want)
		}
	}
}
