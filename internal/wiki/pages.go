package wiki

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	"github.com/butvinm/yandex-skill/internal/render"
)

// page_type values returned by the Yandex Wiki API. Returned unsolicited on
// every /v1/pages response — not in the documented `fields=` allow-list, but
// always present at the top level of the JSON.
const (
	PageTypeWysiwyg = "wysiwyg" // modern Yandex Flavored Markdown
	PageTypePage    = "page"    // legacy "static markup" pages
	PageTypeGrid    = "grid"    // dynamic table; content is null, lives at /v1/grids/{id}
)

type PageAttrs struct {
	ModifiedAt string `json:"modified_at"`
	CreatedAt  string `json:"created_at"`
}

type Page struct {
	ID         int64     `json:"id"`
	Slug       string    `json:"slug"`
	Title      string    `json:"title"`
	PageType   string    `json:"page_type"`
	Content    string    `json:"content"`
	Attributes PageAttrs `json:"attributes"`
}

func (p Page) Plain() string {
	return render.SkipEmptyLines(p.Title, p.Attributes.ModifiedAt, p.Content)
}

type PageRef struct {
	ID    int64  `json:"id"`
	Slug  string `json:"slug"`
	Title string `json:"title,omitempty"`
}

func (p PageRef) Row() string { return render.SkipEmpty(p.Slug, p.Title) }

func (c *Client) GetPage(ctx context.Context, slug string) (*Page, error) {
	q := url.Values{}
	q.Set("slug", slug)
	q.Set("fields", "content")
	var out Page
	_, err := c.Do(ctx, http.MethodGet, "/v1/pages?"+q.Encode(), nil, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// getPageTitle fetches just the page title (omits the heavy `fields=content`
// query parameter). Used by ListPages's title-enrichment fan-out.
func (c *Client) getPageTitle(ctx context.Context, slug string) (string, error) {
	q := url.Values{}
	q.Set("slug", slug)
	var out struct {
		Title string `json:"title"`
	}
	if _, err := c.Do(ctx, http.MethodGet, "/v1/pages?"+q.Encode(), nil, &out); err != nil {
		return "", err
	}
	return out.Title, nil
}

type descendantsPage struct {
	Results    []PageRef `json:"results"`
	NextCursor string    `json:"next_cursor"`
}

func (c *Client) ListPages(ctx context.Context, parent string) ([]PageRef, error) {
	if parent == "" {
		return nil, errors.New("--parent is required for wiki pages list")
	}
	var all []PageRef
	cursor := ""
	for {
		q := url.Values{}
		q.Set("slug", parent)
		q.Set("page_size", "100")
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var page descendantsPage
		_, err := c.Do(ctx, http.MethodGet, "/v1/pages/descendants?"+q.Encode(), nil, &page)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Results...)
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	c.enrichTitles(ctx, all)
	return all, nil
}

// enrichTitles fans out one title fetch per descendant. The
// /v1/pages/descendants endpoint returns id+slug only, but slug-only
// listings are ambiguous (e.g. "ai-services/ai-serv" titled "AI Services
// old" — an LLM can't tell that's the outdated page from the slug alone).
//
// Failures are silent: a per-page fetch error leaves Title empty and the
// row renders as slug-only. Callers see partial enrichment instead of an
// aborted list. Bounded at 10 concurrent fetches to keep large trees
// under control without hammering the API.
func (c *Client) enrichTitles(ctx context.Context, refs []PageRef) {
	const maxConcurrent = 10
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	for i := range refs {
		i := i
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if title, err := c.getPageTitle(ctx, refs[i].Slug); err == nil {
				refs[i].Title = title
			}
		}()
	}
	wg.Wait()
}

type createPageBody struct {
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

func (c *Client) CreatePage(ctx context.Context, slug, title, content string) (*Page, error) {
	body := createPageBody{Slug: slug, Title: title, Content: content}
	var out Page
	_, err := c.Do(ctx, http.MethodPost, "/v1/pages?is_silent=true", body, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

type updatePageBody struct {
	Content string `json:"content"`
}

// UpdatePage updates a page's content. Wiki API takes numeric id in path,
// not slug, so this is a two-step: resolve slug → id, then POST.
func (c *Client) UpdatePage(ctx context.Context, slug, content string) (*Page, error) {
	existing, err := c.GetPage(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("resolve slug: %w", err)
	}
	body := updatePageBody{Content: content}
	var out Page
	_, err = c.Do(ctx, http.MethodPost, fmt.Sprintf("/v1/pages/%d?is_silent=true", existing.ID), body, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}
