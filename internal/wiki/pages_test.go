package wiki

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/butvinm/yandex-skill/internal/auth"
)

func newWiki(t *testing.T, h http.HandlerFunc) (*Client, *httptest.Server) {
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New(auth.Config{Token: "tok", OrgID: "org", WikiBaseURL: srv.URL}), srv
}

func TestPage_Plain(t *testing.T) {
	p := Page{Title: "T", Content: "Body", Attributes: PageAttrs{ModifiedAt: "2026-04-29"}}
	got := p.Plain()
	want := "T\n2026-04-29\nBody"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestPage_Plain_SkipsEmpty(t *testing.T) {
	p := Page{Title: "T", Content: "Body"}
	got := p.Plain()
	want := "T\nBody"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestPageRef_Row(t *testing.T) {
	cases := []struct {
		name string
		in   PageRef
		want string
	}{
		{"slug only when title fetch failed", PageRef{ID: 1, Slug: "team/notes"}, "team/notes"},
		{"slug + title", PageRef{ID: 1, Slug: "team/notes", Title: "Notes"}, "team/notes  Notes"},
	}
	for _, tc := range cases {
		if got := tc.in.Row(); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestGetPage_RequestsFieldsContent(t *testing.T) {
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/pages" {
			t.Errorf("path = %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("slug") != "team/notes" {
			t.Errorf("slug = %s", q.Get("slug"))
		}
		if q.Get("fields") != "content" {
			t.Errorf("fields must be 'content' (otherwise body is omitted by API), got %q", q.Get("fields"))
		}
		_, _ = io.WriteString(w, `{"id":42,"slug":"team/notes","title":"Notes","content":"hello","attributes":{"modified_at":"2026-04-29"}}`)
	})

	got, err := c.GetPage(context.Background(), "team/notes")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != 42 || got.Title != "Notes" || got.Content != "hello" || got.Attributes.ModifiedAt != "2026-04-29" {
		t.Errorf("got = %+v", got)
	}
}

func TestListPages_PaginatesAndEnrichesTitles(t *testing.T) {
	descCalls := 0
	titleCalls := map[string]int{}
	var mu sync.Mutex
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/pages/descendants":
			mu.Lock()
			descCalls++
			mu.Unlock()
			q := r.URL.Query()
			if q.Get("slug") != "team" {
				t.Errorf("slug = %s", q.Get("slug"))
			}
			if descCalls == 1 {
				if q.Get("cursor") != "" {
					t.Errorf("first call should have no cursor, got %q", q.Get("cursor"))
				}
				_, _ = io.WriteString(w, `{"results":[{"id":1,"slug":"team/a"}],"next_cursor":"c2"}`)
				return
			}
			if q.Get("cursor") != "c2" {
				t.Errorf("second call cursor = %q", q.Get("cursor"))
			}
			_, _ = io.WriteString(w, `{"results":[{"id":2,"slug":"team/b"}]}`)
		case "/v1/pages":
			slug := r.URL.Query().Get("slug")
			if r.URL.Query().Get("fields") == "content" {
				t.Errorf("title-only fetch must not request fields=content (slug=%s)", slug)
			}
			mu.Lock()
			titleCalls[slug]++
			mu.Unlock()
			titles := map[string]string{
				"team/a": "Alpha",
				"team/b": "Beta",
			}
			fmt.Fprintf(w, `{"id":0,"slug":%q,"title":%q}`, slug, titles[slug])
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})

	got, err := c.ListPages(context.Background(), "team")
	if err != nil {
		t.Fatal(err)
	}
	if descCalls != 2 {
		t.Errorf("descendant calls = %d", descCalls)
	}
	if titleCalls["team/a"] != 1 || titleCalls["team/b"] != 1 {
		t.Errorf("title calls = %v", titleCalls)
	}
	if len(got) != 2 {
		t.Fatalf("got = %+v", got)
	}
	want := []PageRef{
		{ID: 1, Slug: "team/a", Title: "Alpha"},
		{ID: 2, Slug: "team/b", Title: "Beta"},
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("got[%d] = %+v, want %+v", i, got[i], w)
		}
	}
}

func TestListPages_TitleFetchFailureIsSoft(t *testing.T) {
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/pages/descendants":
			_, _ = io.WriteString(w, `{"results":[{"id":1,"slug":"team/a"},{"id":2,"slug":"team/b"}]}`)
		case "/v1/pages":
			slug := r.URL.Query().Get("slug")
			if slug == "team/a" {
				w.WriteHeader(404)
				_, _ = io.WriteString(w, `{"detail":"gone"}`)
				return
			}
			fmt.Fprintf(w, `{"id":0,"slug":%q,"title":"Beta"}`, slug)
		}
	})

	got, err := c.ListPages(context.Background(), "team")
	if err != nil {
		t.Fatalf("ListPages must not fail when an individual title fetch errors: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got = %+v", got)
	}
	if got[0].Title != "" {
		t.Errorf("failed title fetch should leave Title empty, got %q", got[0].Title)
	}
	if got[1].Title != "Beta" {
		t.Errorf("got[1].Title = %q", got[1].Title)
	}
}

func TestListPages_RequiresParent(t *testing.T) {
	c := New(auth.Config{Token: "t", OrgID: "o"})
	_, err := c.ListPages(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "--parent") {
		t.Fatalf("err = %v", err)
	}
}

func TestCreatePage(t *testing.T) {
	var sentBody createPageBody
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/v1/pages" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("is_silent") != "true" {
			t.Errorf("is_silent should be true, got %q", r.URL.Query().Get("is_silent"))
		}
		_ = json.NewDecoder(r.Body).Decode(&sentBody)
		_, _ = io.WriteString(w, `{"id":99,"slug":"team/new","title":"New"}`)
	})

	got, err := c.CreatePage(context.Background(), "team/new", "New", "body")
	if err != nil {
		t.Fatal(err)
	}
	if sentBody.Slug != "team/new" || sentBody.Title != "New" || sentBody.Content != "body" {
		t.Errorf("sent = %+v", sentBody)
	}
	if got.ID != 99 {
		t.Errorf("got = %+v", got)
	}
}

func TestUpdatePage_TwoStep(t *testing.T) {
	var calls []string
	var sentBody updatePageBody
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			calls = append(calls, "GET "+r.URL.Path+"?"+r.URL.RawQuery)
			_, _ = io.WriteString(w, `{"id":42,"slug":"team/notes","title":"Notes"}`)
		case http.MethodPost:
			calls = append(calls, "POST "+r.URL.Path)
			_ = json.NewDecoder(r.Body).Decode(&sentBody)
			_, _ = io.WriteString(w, `{"id":42,"slug":"team/notes","title":"Notes","content":"new body"}`)
		}
	})

	got, err := c.UpdatePage(context.Background(), "team/notes", "new body")
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %v", calls)
	}
	if !strings.HasPrefix(calls[0], "GET /v1/pages?") {
		t.Errorf("first call = %s", calls[0])
	}
	if calls[1] != "POST /v1/pages/42" {
		t.Errorf("second call = %s", calls[1])
	}
	if sentBody.Content != "new body" {
		t.Errorf("body = %+v", sentBody)
	}
	if got.Content != "new body" {
		t.Errorf("got = %+v", got)
	}
}

func TestUpdatePage_ResolveFails(t *testing.T) {
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = io.WriteString(w, `{"detail":"not found"}`)
	})
	_, err := c.UpdatePage(context.Background(), "missing", "x")
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "resolve slug") {
		t.Errorf("err = %v", err)
	}
}
