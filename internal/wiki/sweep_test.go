//go:build sweep

package wiki

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butvinm/yandex-skill/internal/auth"
)

// TestSweepLegacyAttachments enumerates every page in the org, fetches each
// with content, and reports any non-wysiwyg page whose content contains
// "/.files/" — i.e. would be touched by the markdown-feature regex.
//
// Run with: go test -tags=sweep -run TestSweepLegacyAttachments -v -timeout=10m ./internal/wiki
func TestSweepLegacyAttachments(t *testing.T) {
	cfg, err := auth.Load()
	if err != nil {
		t.Fatal(err)
	}
	c := New(cfg)
	c.http.Timeout = 5 * time.Minute

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	q := url.Values{}
	q.Set("slug", "")
	q.Set("page_size", "100")
	var slugs []string
	cursor := ""
	for {
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var page descendantsPage
		if _, err := c.Do(ctx, http.MethodGet, "/v1/pages/descendants?"+q.Encode(), nil, &page); err != nil {
			t.Fatal(err)
		}
		for _, r := range page.Results {
			slugs = append(slugs, r.Slug)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	fmt.Fprintf(os.Stderr, "SWEEP found %d slugs\n", len(slugs))

	type hit struct {
		slug     string
		pageType string
		excerpt  string
	}
	var (
		mu        sync.Mutex
		hits      []hit
		ptCounts  = map[string]int{}
		fetchErr  int
		otherEx   []string // example slugs for non-wysiwyg/page page_types
	)
	const concurrency = 10
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, s := range slugs {
		s := s
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			pq := url.Values{}
			pq.Set("slug", s)
			pq.Set("fields", "content")
			var raw map[string]any
			if _, err := c.Do(ctx, http.MethodGet, "/v1/pages?"+pq.Encode(), nil, &raw); err != nil {
				mu.Lock()
				fetchErr++
				mu.Unlock()
				return
			}
			pt, _ := raw["page_type"].(string)
			content, _ := raw["content"].(string)
			mu.Lock()
			defer mu.Unlock()
			ptCounts[pt]++
			if pt != "wysiwyg" && pt != "page" && len(otherEx) < 8 {
				otherEx = append(otherEx, fmt.Sprintf("%s=%s", pt, s))
			}
			if pt != "wysiwyg" && strings.Contains(content, "/.files/") {
				ex := content
				if i := strings.Index(content, "/.files/"); i >= 0 {
					start := i - 60
					if start < 0 {
						start = 0
					}
					end := i + 80
					if end > len(content) {
						end = len(content)
					}
					ex = content[start:end]
				}
				hits = append(hits, hit{slug: s, pageType: pt, excerpt: ex})
			}
		}()
	}
	wg.Wait()

	fmt.Fprintf(os.Stderr, "SWEEP page_type counts: %v errors=%d\n", ptCounts, fetchErr)
	fmt.Fprintf(os.Stderr, "SWEEP other-type examples: %v\n", otherEx)
	fmt.Fprintf(os.Stderr, "SWEEP non-wysiwyg with /.files/: %d\n", len(hits))
	for _, h := range hits {
		fmt.Fprintf(os.Stderr, "  HIT slug=%s page_type=%s\n    excerpt=%q\n", h.slug, h.pageType, h.excerpt)
	}
}
