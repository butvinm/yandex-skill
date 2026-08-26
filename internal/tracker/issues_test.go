package tracker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/butvinm/yandex-skill/internal/auth"
)

// newIssue builds an Issue by JSON-encoding each value, mirroring how the
// real decoder populates the map, rather than a Go struct literal.
func newIssue(t *testing.T, fields map[string]any) Issue {
	t.Helper()
	i := Issue{}
	for k, v := range fields {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		i[k] = b
	}
	return i
}

func TestIssue_Plain(t *testing.T) {
	i := newIssue(t, map[string]any{
		"key":         "FOO-1",
		"summary":     "fix it",
		"status":      map[string]string{"display": "Open"},
		"assignee":    map[string]string{"display": "ivan"},
		"updatedAt":   "2026-04-29T10:00Z",
		"description": "do the thing",
	})
	got := i.Plain()
	want := "FOO-1: fix it\nOpen  ivan  2026-04-29T10:00Z\ndo the thing"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestIssue_Plain_SkipsEmpty(t *testing.T) {
	i := newIssue(t, map[string]any{
		"key":     "FOO-1",
		"summary": "no body",
		"status":  map[string]string{"display": "Open"},
	})
	got := i.Plain()
	want := "FOO-1: no body\nOpen"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestIssue_Plain_ExtraFields(t *testing.T) {
	i := newIssue(t, map[string]any{
		"key":            "FOO-1",
		"summary":        "fix it",
		"status":         map[string]string{"display": "Open"},
		"fixVersions":    []map[string]string{{"id": "397", "display": "v1.2.0"}},
		"releaseVersion": "24.3.1",
	})
	got := i.Plain()
	want := "FOO-1: fix it\nOpen\nfixVersions: [{\"display\":\"v1.2.0\",\"id\":\"397\"}]\nreleaseVersion: 24.3.1"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestIssue_Row(t *testing.T) {
	i := newIssue(t, map[string]any{
		"key":      "FOO-1",
		"summary":  "fix it",
		"status":   map[string]string{"display": "Open"},
		"assignee": map[string]string{"display": "ivan"},
	})
	got := i.Row()
	want := "FOO-1  Open  ivan  fix it"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestGetIssue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/issues/FOO-1" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"key":"FOO-1","summary":"hi","status":{"display":"Open"},"assignee":{"display":"ivan"},"updatedAt":"X","description":"D"}`)
	}))
	defer srv.Close()
	c := New(auth.Config{Token: "t", OrgID: "o", TrackerBaseURL: srv.URL})

	got, err := c.GetIssue(context.Background(), "FOO-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.str("key") != "FOO-1" || got.display("status") != "Open" || got.display("assignee") != "ivan" {
		t.Errorf("got = %+v", got)
	}
}

func TestGetIssue_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = io.WriteString(w, `{"errorMessages":["Issue not found"]}`)
	}))
	defer srv.Close()
	c := New(auth.Config{Token: "t", OrgID: "o", TrackerBaseURL: srv.URL})

	_, err := c.GetIssue(context.Background(), "FOO-99")
	if err == nil {
		t.Fatal("want error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 404 {
		t.Errorf("err = %v", err)
	}
}

func TestListIssues_RequiresQueueOrQuery(t *testing.T) {
	c := New(auth.Config{Token: "t", OrgID: "o"})
	_, err := c.ListIssues(context.Background(), "", "")
	if err == nil || !strings.Contains(err.Error(), "specify --queue or --query") {
		t.Fatalf("err = %v", err)
	}
}

func TestListIssues_QueueBody(t *testing.T) {
	var sentBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/v3/issues/_search" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&sentBody)
		_, _ = io.WriteString(w, `[{"key":"FOO-1","summary":"x","status":{"display":"Open"}}]`)
	}))
	defer srv.Close()
	c := New(auth.Config{Token: "t", OrgID: "o", TrackerBaseURL: srv.URL})

	issues, err := c.ListIssues(context.Background(), "FOO", "")
	if err != nil {
		t.Fatal(err)
	}
	if sentBody["queue"] != "FOO" {
		t.Errorf("body = %v", sentBody)
	}
	if _, has := sentBody["query"]; has {
		t.Errorf("query should not be set when queue given: %v", sentBody)
	}
	if len(issues) != 1 || issues[0].str("key") != "FOO-1" {
		t.Errorf("issues = %+v", issues)
	}
}

func TestListIssues_QueryBody(t *testing.T) {
	var sentBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&sentBody)
		_, _ = io.WriteString(w, `[]`)
	}))
	defer srv.Close()
	c := New(auth.Config{Token: "t", OrgID: "o", TrackerBaseURL: srv.URL})

	_, err := c.ListIssues(context.Background(), "", `Status: Open`)
	if err != nil {
		t.Fatal(err)
	}
	if sentBody["query"] != "Status: Open" {
		t.Errorf("body = %v", sentBody)
	}
}
