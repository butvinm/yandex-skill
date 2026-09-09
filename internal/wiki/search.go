package wiki

import (
	"context"
	"net/http"
	"strings"

	"github.com/butvinm/yandex-skill/internal/render"
)

// Search result `type` values returned by POST /v1/search.
const (
	SearchTypePage = "page"
	SearchTypeFile = "file"
)

type SearchOpts struct {
	Query   string
	Limit   int    // 1..50; 0 lets the API default (10) apply
	OrderBy string // relevancy | creation_date | modified_date; "" = relevancy
	Type    string // page | file; "" = both
}

type SearchResult struct {
	URL        string `json:"url"`
	Slug       string `json:"slug"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	Type       string `json:"type"`
	ModifiedAt string `json:"modified_at"`
}

func (r SearchResult) Row() string {
	var kind string
	if r.Type == SearchTypeFile {
		kind = "[file]"
	}
	return render.SkipEmpty(r.URL, kind, r.Title, r.ModifiedAt, oneLine(r.Content))
}

// oneLine collapses the search snippet onto a single line so a result stays one row in plain output.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

type searchFilters struct {
	Type string `json:"type,omitempty"`
}

type searchBody struct {
	Query   string         `json:"query"`
	Limit   int            `json:"limit,omitempty"`
	OrderBy string         `json:"order_by,omitempty"`
	Filters *searchFilters `json:"filters,omitempty"`
}

type searchPage struct {
	Results []SearchResult `json:"results"`
}

// Search runs a full-text search across the organization's wiki and returns the first page of ranked hits.
// Unlike the list commands it does not walk the cursor: search is ranked, so callers want the top N, not everything.
func (c *Client) Search(ctx context.Context, opts SearchOpts) ([]SearchResult, error) {
	body := searchBody{Query: opts.Query, Limit: opts.Limit, OrderBy: opts.OrderBy}
	if opts.Type != "" {
		body.Filters = &searchFilters{Type: opts.Type}
	}
	var out searchPage
	if _, err := c.Do(ctx, http.MethodPost, "/v1/search", body, &out); err != nil {
		return nil, err
	}
	for i := range out.Results {
		out.Results[i].URL = c.PageURL(out.Results[i].Slug)
	}
	return out.Results, nil
}
