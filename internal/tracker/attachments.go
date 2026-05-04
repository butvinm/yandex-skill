package tracker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/butvinm/yandex-skill/internal/render"
)

// Attachment is a file attached to an issue. Per Tracker docs, the issue-level
// /attachments listing returns *both* files attached directly to the issue and
// files attached inside comments — there is no flag distinguishing them.
type Attachment struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Size      int64   `json:"size"`
	Mimetype  string  `json:"mimetype"`
	Content   string  `json:"content"`
	CreatedBy Display `json:"createdBy"`
	CreatedAt string  `json:"createdAt"`
}

// Plain emits "id  name  mime  size  created_at  content_url".
// The id is leading so users can copy it for `tracker attachments download`;
// the URL trails so `awk '{print $NF}'` extracts it cleanly.
func (a Attachment) Plain() string {
	return render.SkipEmpty(a.ID, a.Name, a.Mimetype, humanSize(a.Size), a.CreatedAt, a.Content)
}

func (a Attachment) Row() string { return a.Plain() }

func humanSize(bytes int64) string {
	const (
		kib = 1024
		mib = 1024 * 1024
		gib = 1024 * 1024 * 1024
	)
	switch {
	case bytes <= 0:
		return ""
	case bytes < kib:
		return fmt.Sprintf("%d B", bytes)
	case bytes < mib:
		return fmt.Sprintf("%.1f KiB", float64(bytes)/kib)
	case bytes < gib:
		return fmt.Sprintf("%.1f MiB", float64(bytes)/mib)
	default:
		return fmt.Sprintf("%.1f GiB", float64(bytes)/gib)
	}
}

// ListAttachments fetches all attachments on an issue. The Tracker listing
// includes both issue-level files and files attached inside comments — no
// filter or flag is needed (or available).
func (c *Client) ListAttachments(ctx context.Context, issueKey string) ([]Attachment, error) {
	var all []Attachment
	err := c.DoPaginated(ctx, "/v3/issues/"+issueKey+"/attachments", nil,
		func(raw []byte) error {
			var batch []Attachment
			if err := json.Unmarshal(raw, &batch); err != nil {
				return fmt.Errorf("decode attachments page: %w", err)
			}
			all = append(all, batch...)
			return nil
		})
	if err != nil {
		return nil, err
	}
	return all, nil
}
