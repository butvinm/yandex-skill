package wiki

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/butvinm/yandex-skill/internal/render"
)

type PageAttrs struct {
	ModifiedAt string `json:"modified_at"`
	CreatedAt  string `json:"created_at"`
}

type Page struct {
	ID         int64     `json:"id"`
	Slug       string    `json:"slug"`
	Title      string    `json:"title"`
	Content    string    `json:"content"`
	Attributes PageAttrs `json:"attributes"`
}

func (p Page) Plain() string {
	return render.SkipEmptyLines(p.Title, p.Attributes.ModifiedAt, p.Content)
}

type PageRef struct {
	ID   int64  `json:"id"`
	Slug string `json:"slug"`
}

func (p PageRef) Row() string { return p.Slug }

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
			return all, nil
		}
		cursor = page.NextCursor
	}
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
