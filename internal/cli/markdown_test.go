package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/butvinm/yandex-skill/internal/auth"
	"github.com/butvinm/yandex-skill/internal/wiki"
)

func TestRewriteServerToLocal(t *testing.T) {
	cases := []struct {
		name    string
		content string
		slug    string
		dir     string
		want    string
	}{
		{
			name:    "yfm image with size hint",
			content: "![изображение.png](/users/m/test/.files/img.png =375x383)",
			slug:    "users/m/test", dir: "att",
			want: "![изображение.png](att/img.png =375x383)",
		},
		{
			name:    "yfm file directive",
			content: `:file[name](/users/m/test/.files/foo.png){type="image/png"}`,
			slug:    "users/m/test", dir: "att",
			want: `:file[name](att/foo.png){type="image/png"}`,
		},
		{
			name:    "legacy 0x0 image",
			content: "0x0:/users/m/test/.files/x.png",
			slug:    "users/m/test", dir: "att",
			want: "0x0:att/x.png",
		},
		{
			name:    "cross-page reference left untouched",
			content: "![foo](/other/page/.files/x.png)",
			slug:    "users/m/test", dir: "att",
			want: "![foo](/other/page/.files/x.png)",
		},
		{
			name:    "multiple matches in one content",
			content: "![a](/s/.files/a.png)\n![b](/s/.files/b.png)",
			slug:    "s", dir: "att",
			want: "![a](att/a.png)\n![b](att/b.png)",
		},
		{
			name:    "transliterated filename with collision suffix",
			content: "![](/users/m/test/.files/izobrazhenie-1.png =256x256)",
			slug:    "users/m/test", dir: "att",
			want: "![](att/izobrazhenie-1.png =256x256)",
		},
		{
			name: "empty content", content: "",
			slug: "users/m/test", dir: "att",
			want: "",
		},
		{
			name:    "no matches in plain prose",
			content: "hello world\nthis is markdown",
			slug:    "users/m/test", dir: "att",
			want: "hello world\nthis is markdown",
		},
		{
			name:    "slug with regex metacharacters is escaped",
			content: "![](/a.b/c+d/.files/foo.png)",
			slug:    "a.b/c+d", dir: "att",
			want: "![](att/foo.png)",
		},
		{
			name:    "trailing slash in attachmentsDir is normalized",
			content: "![](/s/.files/x.png)",
			slug:    "s", dir: "att/",
			want: "![](att/x.png)",
		},
		{
			name:    "absolute attachmentsDir preserved",
			content: "![](/s/.files/x.png)",
			slug:    "s", dir: "/tmp/att",
			want: "![](/tmp/att/x.png)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rewriteServerToLocal(tc.content, tc.slug, tc.dir)
			if got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestFindLocalAttachmentRefs(t *testing.T) {
	cases := []struct {
		name    string
		content string
		dir     string
		want    []string
	}{
		{
			name:    "image syntax",
			content: "![alt](att/foo.png)",
			dir:     "att",
			want:    []string{"foo.png"},
		},
		{
			name:    "file directive",
			content: `:file[name](att/doc.pdf){type="application/pdf"}`,
			dir:     "att",
			want:    []string{"doc.pdf"},
		},
		{
			name:    "mixed image and directive",
			content: "![](att/a.png) text :file[](att/b.pdf){type=\"x\"}",
			dir:     "att",
			want:    []string{"a.png", "b.pdf"},
		},
		{
			name:    "duplicates collapsed to unique",
			content: "![](att/x.png) ![](att/x.png) ![](att/y.png)",
			dir:     "att",
			want:    []string{"x.png", "y.png"},
		},
		{
			name:    "no matches",
			content: "plain text with no references",
			dir:     "att",
			want:    []string{},
		},
		{
			name: "empty content", content: "",
			dir:  "att",
			want: []string{},
		},
		{
			name:    "trailing slash in dir is normalized",
			content: "![](att/x.png)",
			dir:     "att/",
			want:    []string{"x.png"},
		},
		{
			name:    "absolute dir",
			content: "![](/tmp/att/x.png)",
			dir:     "/tmp/att",
			want:    []string{"x.png"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := findLocalAttachmentRefs(tc.content, tc.dir)
			// Treat nil and empty-slice as equal for empty-want cases.
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got  %v\nwant %v", got, tc.want)
			}
		})
	}
}

func TestRewriteLocalToServer(t *testing.T) {
	cases := []struct {
		name    string
		content string
		dir     string
		urls    map[string]string
		want    string
	}{
		{
			name:    "image rewritten when basename mapped",
			content: "![](att/foo.png)",
			dir:     "att",
			urls:    map[string]string{"foo.png": "/users/m/test/.files/foo.png"},
			want:    "![](/users/m/test/.files/foo.png)",
		},
		{
			name:    "file directive rewritten",
			content: `:file[name](att/doc.pdf){type="application/pdf"}`,
			dir:     "att",
			urls:    map[string]string{"doc.pdf": "/users/m/test/.files/doc.pdf"},
			want:    `:file[name](/users/m/test/.files/doc.pdf){type="application/pdf"}`,
		},
		{
			name:    "missing basename left untouched",
			content: "![](att/foo.png) ![](att/bar.png)",
			dir:     "att",
			urls:    map[string]string{"foo.png": "/s/.files/foo.png"},
			want:    "![](/s/.files/foo.png) ![](att/bar.png)",
		},
		{
			name:    "empty url map is a no-op",
			content: "![](att/foo.png)",
			dir:     "att",
			urls:    map[string]string{},
			want:    "![](att/foo.png)",
		},
		{
			name:    "multiple basenames all rewritten",
			content: "![](att/a.png)\n![](att/b.png)",
			dir:     "att",
			urls: map[string]string{
				"a.png": "/s/.files/a.png",
				"b.png": "/s/.files/b.png",
			},
			want: "![](/s/.files/a.png)\n![](/s/.files/b.png)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rewriteLocalToServer(tc.content, tc.dir, tc.urls)
			if got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestRefuseGrid(t *testing.T) {
	if err := refuseGrid(wiki.PageTypeWysiwyg); err != nil {
		t.Errorf("wysiwyg should not refuse: %v", err)
	}
	if err := refuseGrid(wiki.PageTypePage); err != nil {
		t.Errorf("page should not refuse: %v", err)
	}
	if err := refuseGrid(""); err != nil {
		t.Errorf("unknown should not refuse: %v", err)
	}
	err := refuseGrid(wiki.PageTypeGrid)
	if err == nil || !strings.Contains(err.Error(), "page_type=grid") {
		t.Errorf("grid should refuse with helpful error, got %v", err)
	}
}

func TestWarnNonWysiwyg(t *testing.T) {
	cases := []struct {
		pageType string
		wantWarn bool
	}{
		{wiki.PageTypeWysiwyg, false},
		{wiki.PageTypeGrid, false}, // grid is filtered by refuseGrid; warn shouldn't fire
		{wiki.PageTypePage, true},
		{"", true},
		{"unknown_future", true},
	}
	for _, tc := range cases {
		t.Run(tc.pageType, func(t *testing.T) {
			var buf bytes.Buffer
			warnNonWysiwyg(tc.pageType, &buf)
			gotWarn := buf.Len() > 0
			if gotWarn != tc.wantWarn {
				t.Errorf("warn=%v want=%v stderr=%q", gotWarn, tc.wantWarn, buf.String())
			}
			if gotWarn && !strings.Contains(buf.String(), "warning:") {
				t.Errorf("warning should be prefixed: %q", buf.String())
			}
		})
	}
}

// fakeWikiForGet spins up an httptest server that handles the endpoints
// syncAttachmentsForGet uses: page resolve, attachment list, attachment
// download. Callers pass the page id/slug, the attachments JSON to serve,
// and the byte blob returned for every download_by_url GET.
func fakeWikiForGet(t *testing.T, pageID int64, pageSlug, attsJSON string, blob []byte) *wiki.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/pages":
			fmt.Fprintf(w, `{"id":%d,"slug":%q,"title":"T","page_type":"wysiwyg","content":""}`, pageID, pageSlug)
		case r.URL.Path == fmt.Sprintf("/v1/pages/%d/attachments", pageID):
			io.WriteString(w, attsJSON)
		case r.URL.Path == "/v1/pages/attachments/download_by_url":
			_, _ = w.Write(blob)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return wiki.New(auth.Config{Token: "t", OrgID: "o", WikiBaseURL: srv.URL})
}

func TestSyncAttachmentsForGet_Wysiwyg(t *testing.T) {
	atts := `{"results":[
		{"id":1,"name":"изображение.png","download_url":"/users/m/test/.files/img.png","check_status":"ready"},
		{"id":2,"name":"doc.pdf","download_url":"/users/m/test/.files/doc.pdf","check_status":"ready"}
	]}`
	blob := []byte("BINARY")
	client := fakeWikiForGet(t, 42, "users/m/test", atts, blob)
	page := &wiki.Page{
		ID:       42,
		Slug:     "users/m/test",
		PageType: wiki.PageTypeWysiwyg,
		Content:  "![](/users/m/test/.files/img.png =100x100)\n:file[doc](/users/m/test/.files/doc.pdf){type=\"application/pdf\"}",
	}
	dir := t.TempDir()
	var stderr bytes.Buffer
	got, err := syncAttachmentsForGet(context.Background(), client, page, dir, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if stderr.Len() > 0 {
		t.Errorf("wysiwyg should produce no warning, stderr=%q", stderr.String())
	}
	wantContent := "![](" + dir + "/img.png =100x100)\n:file[doc](" + dir + "/doc.pdf){type=\"application/pdf\"}"
	if got != wantContent {
		t.Errorf("content =\n%q\nwant\n%q", got, wantContent)
	}
	for _, name := range []string{"img.png", "doc.pdf"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("read %s: %v", name, err)
		} else if string(b) != "BINARY" {
			t.Errorf("file %s contents = %q", name, string(b))
		}
	}
}

func TestSyncAttachmentsForGet_Grid_Refuses(t *testing.T) {
	page := &wiki.Page{Slug: "x", PageType: wiki.PageTypeGrid}
	var stderr bytes.Buffer
	_, err := syncAttachmentsForGet(context.Background(), nil, page, t.TempDir(), &stderr)
	if err == nil || !strings.Contains(err.Error(), "page_type=grid") {
		t.Fatalf("want grid refusal, got %v", err)
	}
}

func TestSyncAttachmentsForGet_Page_Warns(t *testing.T) {
	atts := `{"results":[]}`
	client := fakeWikiForGet(t, 7, "homepage", atts, nil)
	page := &wiki.Page{
		ID:       7,
		Slug:     "homepage",
		PageType: wiki.PageTypePage,
		Content:  "((http://x Title))",
	}
	var stderr bytes.Buffer
	got, err := syncAttachmentsForGet(context.Background(), client, page, t.TempDir(), &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "warning:") || !strings.Contains(stderr.String(), `"page"`) {
		t.Errorf("expected warning to stderr, got %q", stderr.String())
	}
	// Content has no /<slug>/.files/ matches, so rewrite is a no-op.
	if got != page.Content {
		t.Errorf("legacy content with no /.files/ should pass through unchanged, got %q", got)
	}
}

