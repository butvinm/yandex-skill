package tracker

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/butvinm/yandex-skill/internal/render"
)

// CommentAttachmentRef is the inline reference Tracker returns inside a
// comment when the comments endpoint is queried with ?expand=attachments.
// It is *not* a full Attachment — only id + filename. To get size, mimetype,
// or a download URL, cross-reference the id against ListAttachments.
type CommentAttachmentRef struct {
	ID      string `json:"id"`
	Display string `json:"display"`
}

type Comment struct {
	ID          int64                  `json:"id"`
	LongID      string                 `json:"longId"`
	Text        string                 `json:"text"`
	CreatedBy   Display                `json:"createdBy"`
	CreatedAt   string                 `json:"createdAt"`
	UpdatedAt   string                 `json:"updatedAt"`
	Attachments []CommentAttachmentRef `json:"attachments,omitempty"`
}

func (c Comment) Plain() string {
	header := render.SkipEmpty(c.CreatedBy.Display, c.CreatedAt)
	var attachLine string
	if len(c.Attachments) > 0 {
		parts := make([]string, len(c.Attachments))
		for i, a := range c.Attachments {
			parts[i] = a.ID + ":" + a.Display
		}
		attachLine = "attachments: " + strings.Join(parts, ", ")
	}
	return render.SkipEmptyLines(header, c.Text, attachLine)
}

func (c Comment) Row() string {
	base := render.SkipEmpty(c.CreatedBy.Display, c.CreatedAt, firstLine(c.Text))
	if n := len(c.Attachments); n > 0 {
		return base + "  [" + strconv.Itoa(n) + " attached]"
	}
	return base
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i != -1 {
		return s[:i]
	}
	return s
}

// ListComments fetches all comments on an issue. The expand=attachments query
// param is load-bearing: without it, the Attachments field on each comment
// is silently empty even when the comment has files attached.
func (c *Client) ListComments(ctx context.Context, issueKey string) ([]Comment, error) {
	var all []Comment
	err := c.DoPaginated(ctx, "/v3/issues/"+issueKey+"/comments?expand=attachments", nil,
		func(raw []byte) error {
			var batch []Comment
			if err := json.Unmarshal(raw, &batch); err != nil {
				return fmt.Errorf("decode comments page: %w", err)
			}
			all = append(all, batch...)
			return nil
		})
	if err != nil {
		return nil, err
	}
	return all, nil
}
