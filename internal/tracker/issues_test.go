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

	"github.com/butvinm/yandex-cli/internal/auth"
)

func TestIssue_Plain(t *testing.T) {
	i := Issue{
		Key:         "FOO-1",
		Summary:     "fix it",
		Status:      Display{Display: "Open"},
		Assignee:    Display{Display: "ivan"},
		UpdatedAt:   "2026-04-29T10:00Z",
		Description: "do the thing",
	}
	got := i.Plain()
	want := "FOO-1: fix it\nOpen  ivan  2026-04-29T10:00Z\ndo the thing"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestIssue_Plain_SkipsEmpty(t *testing.T) {
	i := Issue{Key: "FOO-1", Summary: "no body", Status: Display{Display: "Open"}}
	got := i.Plain()
	want := "FOO-1: no body\nOpen"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestIssue_Row(t *testing.T) {
	i := Issue{Key: "FOO-1", Summary: "fix it", Status: Display{Display: "Open"}, Assignee: Display{Display: "ivan"}}
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
	if got.Key != "FOO-1" || got.Status.Display != "Open" || got.Assignee.Display != "ivan" {
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
	if len(issues) != 1 || issues[0].Key != "FOO-1" {
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
