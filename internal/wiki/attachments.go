package wiki

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/butvinm/yandex-skill/internal/render"
)

// MaxAttachmentSize caps single-part uploads. The Yandex Wiki upload-sessions
// API accepts parts of 5..16 MiB; we ship single-part only to keep the client
// lean, so we hard-cap at 16 MiB.
const MaxAttachmentSize = 16 * 1024 * 1024

type Attachment struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	Mimetype    string `json:"mimetype"`
	DownloadURL string `json:"download_url"`
	CreatedAt   string `json:"created_at"`
	CheckStatus string `json:"check_status"`
	HasPreview  bool   `json:"has_preview"`
}

func (a Attachment) Plain() string {
	return render.SkipEmpty(a.Name, humanSize(a.Size), a.Mimetype, a.CreatedAt)
}

func (a Attachment) Row() string { return a.Plain() }

func humanSize(n int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.1fGB", float64(n)/float64(gb))
	case n >= mb:
		return fmt.Sprintf("%.1fMB", float64(n)/float64(mb))
	case n >= kb:
		return strconv.FormatInt(n/kb, 10) + "KB"
	default:
		return strconv.FormatInt(n, 10) + "B"
	}
}

type attachmentsPage struct {
	Results    []Attachment `json:"results"`
	NextCursor string       `json:"next_cursor"`
}

func (c *Client) ListAttachments(ctx context.Context, pageSlug string) ([]Attachment, error) {
	page, err := c.GetPage(ctx, pageSlug)
	if err != nil {
		return nil, fmt.Errorf("resolve slug: %w", err)
	}
	var all []Attachment
	cursor := ""
	for {
		q := url.Values{}
		q.Set("page_size", "100")
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var out attachmentsPage
		path := fmt.Sprintf("/v1/pages/%d/attachments?%s", page.ID, q.Encode())
		_, err := c.Do(ctx, http.MethodGet, path, nil, &out)
		if err != nil {
			return nil, err
		}
		all = append(all, out.Results...)
		if out.NextCursor == "" {
			return all, nil
		}
		cursor = out.NextCursor
	}
}
