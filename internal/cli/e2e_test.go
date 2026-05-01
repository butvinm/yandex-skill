package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestE2E_WikiAttachmentsList_Plain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/pages":
			_, _ = io.WriteString(w, `{"id":42,"slug":"team/notes","title":"T"}`)
		case "/v1/pages/42/attachments":
			_, _ = io.WriteString(w, `{"results":[{"id":1,"name":"diagram.png","size":2048,"mimetype":"image/png","created_at":"2026-05-01","check_status":"ready"},{"id":2,"name":"draft.md","size":300,"mimetype":"text/markdown","created_at":"2026-05-01","check_status":"ready"}]}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	stdout, stderr, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":         "tok",
		"YANDEX_CLOUD_ORG_ID":  "org",
		"YANDEX_WIKI_BASE_URL": srv.URL,
	}, "", "wiki", "attachments", "list", "team/notes")

	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr)
	}
	want := "diagram.png  2KB  image/png  2026-05-01\ndraft.md  300B  text/markdown  2026-05-01\n"
	if stdout != want {
		t.Errorf("stdout = %q\nwant      %q", stdout, want)
	}
}

func TestE2E_WikiAttachmentsList_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/pages":
			_, _ = io.WriteString(w, `{"id":42,"slug":"team/notes","title":"T"}`)
		case "/v1/pages/42/attachments":
			_, _ = io.WriteString(w, `{"results":[{"id":1,"name":"diagram.png","size":2048,"mimetype":"image/png","check_status":"ready"}]}`)
		}
	}))
	defer srv.Close()

	stdout, _, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":         "tok",
		"YANDEX_CLOUD_ORG_ID":  "org",
		"YANDEX_WIKI_BASE_URL": srv.URL,
	}, "", "--json", "wiki", "attachments", "list", "team/notes")

	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	for _, want := range []string{`"name": "diagram.png"`, `"size": 2048`, `"check_status": "ready"`} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %s: %q", want, stdout)
		}
	}
}

func TestE2E_WikiAttachmentsDownload_ToFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/pages":
			_, _ = io.WriteString(w, `{"id":42,"slug":"team/notes","title":"T"}`)
		case "/v1/pages/42/attachments":
			_, _ = io.WriteString(w, `{"results":[{"id":1,"name":"diagram.png","check_status":"ready"}]}`)
		case "/v1/pages/attachments/download_by_url":
			_, _ = w.Write([]byte("PNGDATA"))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "diagram.png")
	_, stderr, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":         "tok",
		"YANDEX_CLOUD_ORG_ID":  "org",
		"YANDEX_WIKI_BASE_URL": srv.URL,
	}, "", "wiki", "attachments", "download", "team/notes", "diagram.png", "--output", out)

	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "PNGDATA" {
		t.Errorf("file = %q", got)
	}
}

func TestE2E_WikiAttachmentsDownload_ToStdout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/pages":
			_, _ = io.WriteString(w, `{"id":42,"slug":"team/notes","title":"T"}`)
		case "/v1/pages/42/attachments":
			_, _ = io.WriteString(w, `{"results":[{"id":1,"name":"draft.md","check_status":"ready"}]}`)
		case "/v1/pages/attachments/download_by_url":
			_, _ = w.Write([]byte("# hello"))
		}
	}))
	defer srv.Close()

	stdout, _, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":         "tok",
		"YANDEX_CLOUD_ORG_ID":  "org",
		"YANDEX_WIKI_BASE_URL": srv.URL,
	}, "", "wiki", "attachments", "download", "team/notes", "draft.md")

	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if stdout != "# hello" {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestE2E_WikiAttachmentsUpload_Plain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pages":
			_, _ = io.WriteString(w, `{"id":42,"slug":"team/notes","title":"T"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/upload_sessions":
			_, _ = io.WriteString(w, `{"session_id":"u-1","status":"not_started"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/v1/upload_sessions/u-1/upload_part":
			w.WriteHeader(200)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/upload_sessions/u-1/finish":
			w.WriteHeader(200)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/pages/42/attachments":
			_, _ = io.WriteString(w, `{"results":[{"id":7,"name":"diagram.png"}]}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	src := filepath.Join(t.TempDir(), "diagram.png")
	if err := os.WriteFile(src, []byte("PNGDATA"), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":         "tok",
		"YANDEX_CLOUD_ORG_ID":  "org",
		"YANDEX_WIKI_BASE_URL": srv.URL,
	}, "", "wiki", "attachments", "upload", "team/notes", "--file", src)

	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr)
	}
	if stdout != "uploaded: diagram.png\n" {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestE2E_WikiAttachmentsUpload_JSON_NameOverride(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pages":
			_, _ = io.WriteString(w, `{"id":42,"slug":"team/notes","title":"T"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/upload_sessions":
			var sent map[string]any
			_ = json.NewDecoder(r.Body).Decode(&sent)
			if sent["file_name"] != "renamed.bin" {
				t.Errorf("file_name = %v", sent["file_name"])
			}
			_, _ = io.WriteString(w, `{"session_id":"u-1"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/v1/upload_sessions/u-1/upload_part":
			w.WriteHeader(200)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/upload_sessions/u-1/finish":
			w.WriteHeader(200)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/pages/42/attachments":
			_, _ = io.WriteString(w, `{"results":[{"id":7,"name":"renamed.bin"}]}`)
		}
	}))
	defer srv.Close()

	src := filepath.Join(t.TempDir(), "actual.bin")
	if err := os.WriteFile(src, []byte("DATA"), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, _, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":         "tok",
		"YANDEX_CLOUD_ORG_ID":  "org",
		"YANDEX_WIKI_BASE_URL": srv.URL,
	}, "", "--json", "wiki", "attachments", "upload", "team/notes", "--file", src, "--name", "renamed.bin")

	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if !strings.Contains(stdout, `"uploaded": "renamed.bin"`) {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestE2E_WikiAttachmentsUpload_RequiresFile(t *testing.T) {
	_, stderr, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":         "tok",
		"YANDEX_CLOUD_ORG_ID":  "org",
		"YANDEX_WIKI_BASE_URL": "http://unused",
	}, "", "wiki", "attachments", "upload", "team/notes")

	if exit == 0 {
		t.Fatal("expected non-zero exit when --file is missing")
	}
	if !strings.Contains(stderr, "--file") {
		t.Errorf("stderr should mention --file: %q", stderr)
	}
}

func TestE2E_WikiAttachmentsDelete_Plain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pages":
			_, _ = io.WriteString(w, `{"id":42,"slug":"team/notes","title":"T"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pages/42/attachments":
			_, _ = io.WriteString(w, `{"results":[{"id":7,"name":"old.png"}]}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/pages/42/attachments/7":
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	stdout, stderr, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":         "tok",
		"YANDEX_CLOUD_ORG_ID":  "org",
		"YANDEX_WIKI_BASE_URL": srv.URL,
	}, "", "wiki", "attachments", "delete", "team/notes", "old.png")

	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr)
	}
	if stdout != "deleted: old.png\n" {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestE2E_WikiAttachmentsDelete_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pages":
			_, _ = io.WriteString(w, `{"id":42,"slug":"team/notes","title":"T"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pages/42/attachments":
			_, _ = io.WriteString(w, `{"results":[{"id":7,"name":"old.png"}]}`)
		case r.Method == http.MethodDelete:
			w.WriteHeader(204)
		}
	}))
	defer srv.Close()

	stdout, _, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":         "tok",
		"YANDEX_CLOUD_ORG_ID":  "org",
		"YANDEX_WIKI_BASE_URL": srv.URL,
	}, "", "--json", "wiki", "attachments", "delete", "team/notes", "old.png")

	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if !strings.Contains(stdout, `"deleted": "old.png"`) {
		t.Errorf("stdout = %q", stdout)
	}
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
