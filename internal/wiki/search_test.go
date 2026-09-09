package wiki

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestSearchResult_Row(t *testing.T) {
	cases := []struct {
		name string
		in   SearchResult
		want string
	}{
		{"page", SearchResult{URL: "https://wiki.test/team/notes", Title: "Notes", Type: "page", ModifiedAt: "2026-04-29T10:00:00Z", Content: "first line\n  second\tline"}, "https://wiki.test/team/notes  Notes  2026-04-29T10:00:00Z  first line second line"},
		{"file", SearchResult{URL: "https://wiki.test/team/notes/.files/a.pdf", Title: "a.pdf", Type: "file"}, "https://wiki.test/team/notes/.files/a.pdf  [file]  a.pdf"},
	}
	for _, tc := range cases {
		if got := tc.in.Row(); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestSearch_PostsBodyAndBuildsURLs(t *testing.T) {
	var got map[string]any
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/search" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(w, `{"results":[{"url":"/team/notes","slug":"team/notes","title":"Notes","content":"hit","type":"page","modified_at":"2026-04-29T10:00:00Z"}],"next_cursor":"2"}`)
	})

	res, err := c.Search(context.Background(), SearchOpts{Query: "deploy", Limit: 5, OrderBy: "modified_date", Type: "page"})
	if err != nil {
		t.Fatal(err)
	}
	if got["query"] != "deploy" || got["limit"] != float64(5) || got["order_by"] != "modified_date" {
		t.Errorf("body = %v", got)
	}
	if f, _ := got["filters"].(map[string]any); f["type"] != "page" {
		t.Errorf("filters = %v", got["filters"])
	}
	if len(res) != 1 {
		t.Fatalf("res = %+v", res)
	}
	want := SearchResult{URL: "https://wiki.test/team/notes", Slug: "team/notes", Title: "Notes", Content: "hit", Type: "page", ModifiedAt: "2026-04-29T10:00:00Z"}
	if res[0] != want {
		t.Errorf("got %+v want %+v", res[0], want)
	}
}

func TestSearch_OmitsUnsetOptions(t *testing.T) {
	var got map[string]any
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(w, `{"results":[]}`)
	})
	if _, err := c.Search(context.Background(), SearchOpts{Query: "x"}); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"limit", "order_by", "filters", "cursor", "highlight"} {
		if _, ok := got[k]; ok {
			t.Errorf("body should omit %s: %v", k, got)
		}
	}
}
