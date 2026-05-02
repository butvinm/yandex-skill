package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
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

func TestE2E_WikiPagesGet_OutputFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"id":1,"slug":"team/notes","title":"Notes","page_type":"wysiwyg","content":"# hi\nbody","attributes":{"modified_at":"2026-04-29"}}`)
	}))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "page.md")
	stdout, stderr, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":         "tok",
		"YANDEX_CLOUD_ORG_ID":  "org",
		"YANDEX_WIKI_BASE_URL": srv.URL,
	}, "", "wiki", "pages", "get", "team/notes", "--output", out)

	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr)
	}
	if stdout != "" {
		t.Errorf("--output to file should produce empty stdout, got %q", stdout)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	// Raw content only — no title prefix, no modified_at line.
	if string(got) != "# hi\nbody" {
		t.Errorf("file content = %q", string(got))
	}
}

func TestE2E_WikiPagesGet_OutputDash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"id":1,"slug":"team/notes","title":"Notes","page_type":"wysiwyg","content":"# hi\nbody","attributes":{"modified_at":"2026-04-29"}}`)
	}))
	defer srv.Close()

	stdout, _, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":         "tok",
		"YANDEX_CLOUD_ORG_ID":  "org",
		"YANDEX_WIKI_BASE_URL": srv.URL,
	}, "", "wiki", "pages", "get", "team/notes", "--output", "-")

	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	// --output - emits raw content to stdout (no title prefix, no trailing newline).
	if stdout != "# hi\nbody" {
		t.Errorf("stdout = %q", stdout)
	}
}

// wikiGetMux serves GetPage + ListAttachments + DownloadAttachment for the
// e2e tests of `wiki pages get --attachments-dir`. Customize via the page
// object and attachment list/blob.
func wikiGetMux(t *testing.T, page string, pageID int64, atts string, blob []byte) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/pages":
			_, _ = io.WriteString(w, page)
		case r.URL.Path == "/v1/pages/"+strconv.FormatInt(pageID, 10)+"/attachments":
			_, _ = io.WriteString(w, atts)
		case r.URL.Path == "/v1/pages/attachments/download_by_url":
			_, _ = w.Write(blob)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}
}

func TestE2E_WikiPagesGet_AttachmentsDir_Wysiwyg(t *testing.T) {
	page := `{"id":42,"slug":"users/m/test","title":"T","page_type":"wysiwyg","content":"![alt](/users/m/test/.files/img.png =100x100)\n:file[doc](/users/m/test/.files/doc.pdf){type=\"application/pdf\"}"}`
	atts := `{"results":[
		{"id":1,"name":"изображение.png","download_url":"/users/m/test/.files/img.png","check_status":"ready"},
		{"id":2,"name":"doc.pdf","download_url":"/users/m/test/.files/doc.pdf","check_status":"ready"}
	]}`
	srv := httptest.NewServer(wikiGetMux(t, page, 42, atts, []byte("BLOB")))
	defer srv.Close()

	dir := t.TempDir()
	stdout, stderr, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":         "tok",
		"YANDEX_CLOUD_ORG_ID":  "org",
		"YANDEX_WIKI_BASE_URL": srv.URL,
	}, "", "wiki", "pages", "get", "users/m/test", "--attachments-dir", dir)

	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr)
	}
	if stderr != "" {
		t.Errorf("wysiwyg should produce no stderr, got %q", stderr)
	}
	wantBoth := []string{
		"![alt](" + dir + "/img.png =100x100)",
		":file[doc](" + dir + "/doc.pdf){type=\"application/pdf\"}",
	}
	for _, w := range wantBoth {
		if !strings.Contains(stdout, w) {
			t.Errorf("stdout missing %q\nfull stdout:\n%s", w, stdout)
		}
	}
	for _, name := range []string{"img.png", "doc.pdf"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(b) != "BLOB" {
			t.Errorf("attachment file %s = %q err=%v", name, string(b), err)
		}
	}
}

func TestE2E_WikiPagesGet_AttachmentsDir_CrossPageRefUntouched(t *testing.T) {
	page := `{"id":42,"slug":"a/page","title":"T","page_type":"wysiwyg","content":"![own](/a/page/.files/own.png) ![other](/b/other/.files/other.png)"}`
	atts := `{"results":[{"id":1,"name":"own.png","download_url":"/a/page/.files/own.png","check_status":"ready"}]}`
	srv := httptest.NewServer(wikiGetMux(t, page, 42, atts, []byte("X")))
	defer srv.Close()

	dir := t.TempDir()
	stdout, _, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":         "tok",
		"YANDEX_CLOUD_ORG_ID":  "org",
		"YANDEX_WIKI_BASE_URL": srv.URL,
	}, "", "wiki", "pages", "get", "a/page", "--attachments-dir", dir)

	if exit != 0 {
		t.Fatalf("exit=%d", exit)
	}
	if !strings.Contains(stdout, "![own]("+dir+"/own.png)") {
		t.Errorf("own ref should be rewritten: %q", stdout)
	}
	if !strings.Contains(stdout, "![other](/b/other/.files/other.png)") {
		t.Errorf("cross-page ref should be untouched: %q", stdout)
	}
}

func TestE2E_WikiPagesGet_AttachmentsDir_DuplicateNames(t *testing.T) {
	// All 3 attachments share Name; download_url uses -1, -2 suffix
	// disambiguation. Local files should follow the URL basename, so all
	// three land on disk distinctly.
	page := `{"id":42,"slug":"u/p","title":"T","page_type":"wysiwyg","content":"![](/u/p/.files/img.png) ![](/u/p/.files/img-1.png) ![](/u/p/.files/img-2.png)"}`
	atts := `{"results":[
		{"id":1,"name":"img.png","download_url":"/u/p/.files/img.png","check_status":"ready"},
		{"id":2,"name":"img.png","download_url":"/u/p/.files/img-1.png","check_status":"ready"},
		{"id":3,"name":"img.png","download_url":"/u/p/.files/img-2.png","check_status":"ready"}
	]}`
	srv := httptest.NewServer(wikiGetMux(t, page, 42, atts, []byte("Y")))
	defer srv.Close()

	dir := t.TempDir()
	_, _, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":         "tok",
		"YANDEX_CLOUD_ORG_ID":  "org",
		"YANDEX_WIKI_BASE_URL": srv.URL,
	}, "", "wiki", "pages", "get", "u/p", "--attachments-dir", dir)
	if exit != 0 {
		t.Fatalf("exit=%d", exit)
	}
	for _, n := range []string{"img.png", "img-1.png", "img-2.png"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Errorf("missing file %s: %v", n, err)
		}
	}
}

func TestE2E_WikiPagesGet_AttachmentsDir_Page_WarningOnStderr(t *testing.T) {
	page := `{"id":42,"slug":"homepage","title":"H","page_type":"page","content":"((http://x Title))"}`
	atts := `{"results":[]}`
	srv := httptest.NewServer(wikiGetMux(t, page, 42, atts, nil))
	defer srv.Close()

	_, stderr, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":         "tok",
		"YANDEX_CLOUD_ORG_ID":  "org",
		"YANDEX_WIKI_BASE_URL": srv.URL,
	}, "", "wiki", "pages", "get", "homepage", "--attachments-dir", t.TempDir())

	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr)
	}
	if !strings.Contains(stderr, "warning") || !strings.Contains(stderr, `"page"`) {
		t.Errorf("expected stderr warning for page_type=page, got %q", stderr)
	}
}

func TestE2E_WikiPagesGet_AttachmentsDir_Grid_RefuseWithError(t *testing.T) {
	page := `{"id":42,"slug":"some/grid","title":"G","page_type":"grid","content":null}`
	srv := httptest.NewServer(wikiGetMux(t, page, 42, "", nil))
	defer srv.Close()

	_, stderr, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":         "tok",
		"YANDEX_CLOUD_ORG_ID":  "org",
		"YANDEX_WIKI_BASE_URL": srv.URL,
	}, "", "wiki", "pages", "get", "some/grid", "--attachments-dir", t.TempDir())

	if exit != 1 {
		t.Errorf("expected exit=1, got %d", exit)
	}
	if !strings.Contains(stderr, "page_type=grid") {
		t.Errorf("expected grid refusal in stderr, got %q", stderr)
	}
}

func TestE2E_WikiPagesUpdate_AttachmentsDir_NewFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "foo.png"), []byte("PNG"), 0o644); err != nil {
		t.Fatal(err)
	}
	bodyFile := filepath.Join(dir, "page.md")
	if err := os.WriteFile(bodyFile, []byte("![](" + dir + "/foo.png)"), 0o644); err != nil {
		t.Fatal(err)
	}

	var sentUpdateBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pages":
			_, _ = io.WriteString(w, `{"id":42,"slug":"u/p","title":"T","page_type":"wysiwyg","content":""}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pages/42/attachments":
			_, _ = io.WriteString(w, `{"results":[]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/upload_sessions":
			_, _ = io.WriteString(w, `{"session_id":"u-1","status":"not_started"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/v1/upload_sessions/u-1/upload_part":
			w.WriteHeader(200)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/upload_sessions/u-1/finish":
			w.WriteHeader(200)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/pages/42/attachments":
			_, _ = io.WriteString(w, `{"results":[{"id":7,"name":"foo.png","download_url":"/u/p/.files/foomangled.png","check_status":"ready"}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/pages/42":
			buf, _ := io.ReadAll(r.Body)
			_ = jsonUnmarshal(buf, &sentUpdateBody)
			_, _ = io.WriteString(w, `{"id":42,"slug":"u/p","title":"T","content":""}`)
		default:
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	stdout, stderr, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":         "tok",
		"YANDEX_CLOUD_ORG_ID":  "org",
		"YANDEX_WIKI_BASE_URL": srv.URL,
	}, "", "wiki", "pages", "update", "u/p", "--body-file", bodyFile, "--attachments-dir", dir)

	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr)
	}
	if sentUpdateBody["content"] != "![](/u/p/.files/foomangled.png)" {
		t.Errorf("update content not rewritten: %q", sentUpdateBody["content"])
	}
	if !strings.Contains(stdout, "updated: u/p") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestE2E_WikiPagesUpdate_AttachmentsDir_ExistingFile_Skips(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "foo.png"), []byte("PNG"), 0o644); err != nil {
		t.Fatal(err)
	}
	bodyFile := filepath.Join(dir, "page.md")
	if err := os.WriteFile(bodyFile, []byte("![](" + dir + "/foo.png)"), 0o644); err != nil {
		t.Fatal(err)
	}

	var uploadCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pages":
			_, _ = io.WriteString(w, `{"id":42,"slug":"u/p","title":"T","page_type":"wysiwyg","content":""}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pages/42/attachments":
			_, _ = io.WriteString(w, `{"results":[{"id":1,"name":"foo.png","download_url":"/u/p/.files/foo.png","check_status":"ready"}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/upload_sessions":
			uploadCalled = true
			w.WriteHeader(500)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/pages/42":
			_, _ = io.WriteString(w, `{"id":42,"slug":"u/p","title":"T","content":""}`)
		default:
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	_, stderr, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":         "tok",
		"YANDEX_CLOUD_ORG_ID":  "org",
		"YANDEX_WIKI_BASE_URL": srv.URL,
	}, "", "wiki", "pages", "update", "u/p", "--body-file", bodyFile, "--attachments-dir", dir)

	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr)
	}
	if uploadCalled {
		t.Errorf("upload should be skipped when attachment already exists by basename")
	}
}

func TestE2E_WikiPagesUpdate_AttachmentsDir_Grid_Refuses(t *testing.T) {
	dir := t.TempDir()
	bodyFile := filepath.Join(dir, "page.md")
	if err := os.WriteFile(bodyFile, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"id":42,"slug":"g","title":"G","page_type":"grid","content":null}`)
	}))
	defer srv.Close()

	_, stderr, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":         "tok",
		"YANDEX_CLOUD_ORG_ID":  "org",
		"YANDEX_WIKI_BASE_URL": srv.URL,
	}, "", "wiki", "pages", "update", "g", "--body-file", bodyFile, "--attachments-dir", dir)

	if exit != 1 {
		t.Errorf("exit = %d", exit)
	}
	if !strings.Contains(stderr, "page_type=grid") {
		t.Errorf("expected grid refusal in stderr, got %q", stderr)
	}
}

func TestE2E_WikiPagesCreate_AttachmentsDir_NewPageWithImage(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "img.png"), []byte("PNG"), 0o644); err != nil {
		t.Fatal(err)
	}
	bodyFile := filepath.Join(dir, "page.md")
	if err := os.WriteFile(bodyFile, []byte("# title\n![alt](" + dir + "/img.png =100x100)"), 0o644); err != nil {
		t.Fatal(err)
	}

	var calls []string
	var sentCreateBody, sentUpdateBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		calls = append(calls, key)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/pages":
			buf, _ := io.ReadAll(r.Body)
			_ = jsonUnmarshal(buf, &sentCreateBody)
			_, _ = io.WriteString(w, `{"id":42,"slug":"u/p","title":"T"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pages":
			// Resolve slug → id (needed by ListAttachments inside the orchestrator).
			_, _ = io.WriteString(w, `{"id":42,"slug":"u/p","title":"T","page_type":"wysiwyg","content":""}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pages/42/attachments":
			_, _ = io.WriteString(w, `{"results":[]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/upload_sessions":
			_, _ = io.WriteString(w, `{"session_id":"u-1","status":"not_started"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/v1/upload_sessions/u-1/upload_part":
			w.WriteHeader(200)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/upload_sessions/u-1/finish":
			w.WriteHeader(200)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/pages/42/attachments":
			_, _ = io.WriteString(w, `{"results":[{"id":7,"name":"img.png","download_url":"/u/p/.files/img.png","check_status":"ready"}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/pages/42":
			buf, _ := io.ReadAll(r.Body)
			_ = jsonUnmarshal(buf, &sentUpdateBody)
			_, _ = io.WriteString(w, `{"id":42,"slug":"u/p","title":"T","content":""}`)
		default:
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	stdout, stderr, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":         "tok",
		"YANDEX_CLOUD_ORG_ID":  "org",
		"YANDEX_WIKI_BASE_URL": srv.URL,
	}, "", "wiki", "pages", "create", "--slug", "u/p", "--title", "T", "--body-file", bodyFile, "--attachments-dir", dir)

	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr)
	}
	// Initial create posts an empty body; final update has the rewritten body.
	if sentCreateBody["content"] != "" {
		t.Errorf("initial create should send empty content, got %q", sentCreateBody["content"])
	}
	want := "# title\n![alt](/u/p/.files/img.png =100x100)"
	if sentUpdateBody["content"] != want {
		t.Errorf("update content = %q\nwant %q", sentUpdateBody["content"], want)
	}
	if !strings.Contains(stdout, "created: u/p") {
		t.Errorf("stdout = %q", stdout)
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

func TestE2E_WikiPagesList_PlainIncludesTitles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/pages/descendants":
			_, _ = io.WriteString(w, `{"results":[{"id":1,"slug":"ai-services/ai-serv"},{"id":2,"slug":"ai-services/ai-services-v2"}]}`)
		case "/v1/pages":
			slug := r.URL.Query().Get("slug")
			titles := map[string]string{
				"ai-services/ai-serv":        "AI Services old",
				"ai-services/ai-services-v2": "AI Services",
			}
			_, _ = io.WriteString(w, `{"id":0,"slug":"`+slug+`","title":"`+titles[slug]+`"}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	stdout, stderr, exit := runWithEnv(t, map[string]string{
		"YANDEX_TOKEN":         "tok",
		"YANDEX_CLOUD_ORG_ID":  "org",
		"YANDEX_WIKI_BASE_URL": srv.URL,
	}, "", "wiki", "pages", "list", "--parent", "ai-services")

	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr)
	}
	want := "ai-services/ai-serv  AI Services old\nai-services/ai-services-v2  AI Services\n"
	if stdout != want {
		t.Errorf("stdout = %q\nwant      %q", stdout, want)
	}
}

func TestE2E_WikiAttachmentsList_Plain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/pages":
			_, _ = io.WriteString(w, `{"id":42,"slug":"team/notes","title":"T"}`)
		case "/v1/pages/42/attachments":
			_, _ = io.WriteString(w, `{"results":[{"id":1,"name":"diagram.png","size":"0.00","mimetype":"image/png","download_url":"https://wiki.example/d/1","created_at":"2026-05-01","check_status":"ready"},{"id":2,"name":"draft.md","size":"0.00","mimetype":"text/markdown","download_url":"https://wiki.example/d/2","created_at":"2026-05-01","check_status":"ready"}]}`)
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
	want := "diagram.png  image/png  2026-05-01  https://wiki.example/d/1\ndraft.md  text/markdown  2026-05-01  https://wiki.example/d/2\n"
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
			_, _ = io.WriteString(w, `{"results":[{"id":1,"name":"diagram.png","size":"0.00","mimetype":"image/png","check_status":"ready"}]}`)
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
	for _, want := range []string{`"name": "diagram.png"`, `"size": "0.00"`, `"check_status": "ready"`} {
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
