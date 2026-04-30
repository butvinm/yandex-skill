package wiki

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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
