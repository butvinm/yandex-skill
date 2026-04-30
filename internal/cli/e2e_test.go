package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func runWithEnv(t *testing.T, env map[string]string, stdin string, args ...string) (string, string, int) {
	t.Helper()
	for k, v := range env {
		t.Setenv(k, v)
	}
	var so, se bytes.Buffer
	exit := Run(args, "test", &so, &se, strings.NewReader(stdin))
	return so.String(), se.String(), exit
}

func TestE2E_TrackerIssuesGet_Plain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"key":"FOO-1","summary":"hi","status":{"display":"Open"},"assignee":{"display":"ivan"},"updatedAt":"2026-04-29","description":"do it"}`)
	}))
	defer srv.Close()

	stdout, stderr, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":            "tok",
		"YANDEX_CLOUD_ORG_ID":     "org",
		"YANDEX_TRACKER_BASE_URL": srv.URL,
	}, "", "tracker", "issues", "get", "FOO-1")

	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr)
	}
	want := "FOO-1: hi\nOpen  ivan  2026-04-29\ndo it\n"
	if stdout != want {
		t.Errorf("stdout = %q\nwant      %q", stdout, want)
	}
}

func TestE2E_TrackerIssuesGet_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"key":"FOO-1","summary":"hi","status":{"display":"Open"}}`)
	}))
	defer srv.Close()

	stdout, _, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":            "tok",
		"YANDEX_CLOUD_ORG_ID":     "org",
		"YANDEX_TRACKER_BASE_URL": srv.URL,
	}, "", "--json", "tracker", "issues", "get", "FOO-1")

	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if !strings.Contains(stdout, `"key": "FOO-1"`) {
		t.Errorf("stdout missing key field: %q", stdout)
	}
	if !strings.Contains(stdout, `"display": "Open"`) {
		t.Errorf("stdout missing nested display: %q", stdout)
	}
}

func TestE2E_TrackerIssuesList_Plain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/issues/_search" || r.Method != http.MethodPost {
			t.Errorf("path=%s method=%s", r.URL.Path, r.Method)
		}
		_, _ = io.WriteString(w, `[{"key":"FOO-1","summary":"a","status":{"display":"Open"}},{"key":"FOO-2","summary":"b","status":{"display":"Closed"}}]`)
	}))
	defer srv.Close()

	stdout, stderr, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":            "tok",
		"YANDEX_CLOUD_ORG_ID":     "org",
		"YANDEX_TRACKER_BASE_URL": srv.URL,
	}, "", "tracker", "issues", "list", "--queue", "FOO")

	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr)
	}
	want := "FOO-1  Open  a\nFOO-2  Closed  b\n"
	if stdout != want {
		t.Errorf("stdout = %q\nwant      %q", stdout, want)
	}
}

func TestE2E_WikiPagesGet_Plain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("fields") != "content" {
			t.Errorf("fields query missing: %s", r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `{"id":1,"slug":"team/notes","title":"Notes","content":"# hi","attributes":{"modified_at":"2026-04-29"}}`)
	}))
	defer srv.Close()

	stdout, _, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":         "tok",
		"YANDEX_CLOUD_ORG_ID":  "org",
		"YANDEX_WIKI_BASE_URL": srv.URL,
	}, "", "wiki", "pages", "get", "team/notes")

	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	want := "Notes\n2026-04-29\n# hi\n"
	if stdout != want {
		t.Errorf("stdout = %q\nwant      %q", stdout, want)
	}
}

func TestE2E_WikiPagesCreate_FromBodyFlag(t *testing.T) {
	var sentBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		buf, _ := io.ReadAll(r.Body)
		_ = jsonUnmarshal(buf, &sentBody)
		_, _ = io.WriteString(w, `{"id":1,"slug":"team/new","title":"T"}`)
	}))
	defer srv.Close()

	stdout, _, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":         "tok",
		"YANDEX_CLOUD_ORG_ID":  "org",
		"YANDEX_WIKI_BASE_URL": srv.URL,
	}, "", "wiki", "pages", "create", "--slug", "team/new", "--title", "T", "--body", "hello world")

	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if sentBody["content"] != "hello world" {
		t.Errorf("sent body = %v", sentBody)
	}
	if stdout != "created: team/new\n" {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestE2E_WikiPagesCreate_FromStdin(t *testing.T) {
	var sentBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf, _ := io.ReadAll(r.Body)
		_ = jsonUnmarshal(buf, &sentBody)
		_, _ = io.WriteString(w, `{"id":1,"slug":"team/new","title":"T"}`)
	}))
	defer srv.Close()

	_, _, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":         "tok",
		"YANDEX_CLOUD_ORG_ID":  "org",
		"YANDEX_WIKI_BASE_URL": srv.URL,
	}, "draft from stdin", "wiki", "pages", "create", "--slug", "x", "--title", "T", "--body-file", "-")

	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if sentBody["content"] != "draft from stdin" {
		t.Errorf("sent body = %v", sentBody)
	}
}

func TestE2E_AuthError_404_Plain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = io.WriteString(w, `{"errorMessages":["nope"]}`)
	}))
	defer srv.Close()

	_, stderr, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":            "tok",
		"YANDEX_CLOUD_ORG_ID":     "org",
		"YANDEX_TRACKER_BASE_URL": srv.URL,
	}, "", "tracker", "issues", "get", "X-9")

	if exit != 1 {
		t.Errorf("exit = %d", exit)
	}
	if !strings.Contains(stderr, "(404)") || !strings.Contains(stderr, "nope") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestE2E_AuthError_404_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = io.WriteString(w, `{"errorMessages":["nope"]}`)
	}))
	defer srv.Close()

	_, stderr, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":            "tok",
		"YANDEX_CLOUD_ORG_ID":     "org",
		"YANDEX_TRACKER_BASE_URL": srv.URL,
	}, "", "--json", "tracker", "issues", "get", "X-9")

	if exit != 1 {
		t.Errorf("exit = %d", exit)
	}
	if !strings.Contains(stderr, `"status":404`) {
		t.Errorf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, `"error"`) {
		t.Errorf("stderr = %q", stderr)
	}
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
