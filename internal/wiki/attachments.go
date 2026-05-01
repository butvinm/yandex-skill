package wiki

import (
	"context"
	"fmt"
	"io"
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

type attachReq struct {
	UploadSessions []string `json:"upload_sessions"`
}

type attachResults struct {
	Results []Attachment `json:"results"`
}

func (c *Client) ListAttachments(ctx context.Context, pageSlug string) ([]Attachment, error) {
	page, err := c.GetPage(ctx, pageSlug)
	if err != nil {
		return nil, fmt.Errorf("resolve slug: %w", err)
	}
	return c.listAttachmentsByID(ctx, page.ID)
}

func (c *Client) listAttachmentsByID(ctx context.Context, pageID int64) ([]Attachment, error) {
	var all []Attachment
	cursor := ""
	for {
		q := url.Values{}
		q.Set("page_size", "100")
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var out attachmentsPage
		path := fmt.Sprintf("/v1/pages/%d/attachments?%s", pageID, q.Encode())
		if _, err := c.Do(ctx, http.MethodGet, path, nil, &out); err != nil {
			return nil, err
		}
		all = append(all, out.Results...)
		if out.NextCursor == "" {
			return all, nil
		}
		cursor = out.NextCursor
	}
}

// pickUniqueByName picks the single attachment with the given name from atts.
// Returns one of two stable error messages on miss/duplicate so callers
// share the contract.
func pickUniqueByName(atts []Attachment, pageSlug, filename string) (*Attachment, error) {
	var matches []Attachment
	for _, a := range atts {
		if a.Name == filename {
			matches = append(matches, a)
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("attachment %q not found on page %q", filename, pageSlug)
	case 1:
		return &matches[0], nil
	default:
		return nil, fmt.Errorf("multiple attachments named %q on %q; disambiguate via --json list", filename, pageSlug)
	}
}

// DownloadAttachment streams an attachment's bytes to w. The unique-name
// precondition (and check_status==ready guard) is enforced before issuing
// the binary GET so refusal cases leak no partial data.
func (c *Client) DownloadAttachment(ctx context.Context, pageSlug, filename string, w io.Writer) error {
	atts, err := c.ListAttachments(ctx, pageSlug)
	if err != nil {
		return err
	}
	att, err := pickUniqueByName(atts, pageSlug, filename)
	if err != nil {
		return err
	}
	if att.CheckStatus != "" && att.CheckStatus != "ready" {
		return fmt.Errorf("attachment %q has check_status=%s; refusing to download", filename, att.CheckStatus)
	}
	q := url.Values{}
	q.Set("url", pageSlug+"/"+filename)
	q.Set("download", "true")
	resp, err := c.DoRaw(ctx, http.MethodGet, "/v1/pages/attachments/download_by_url?"+q.Encode(), "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, err = io.Copy(w, resp.Body)
	return err
}

// UploadAttachment ships a file to a wiki page via the 3-step upload-sessions
// protocol. Files larger than MaxAttachmentSize are rejected before any HTTP
// call (we ship single-part uploads only).
func (c *Client) UploadAttachment(ctx context.Context, pageSlug, fileName string, body io.Reader, size int64) (*Attachment, error) {
	if size > MaxAttachmentSize {
		return nil, fmt.Errorf("file too large: %d bytes (max %d MiB single-part upload)", size, MaxAttachmentSize/(1024*1024))
	}
	page, err := c.GetPage(ctx, pageSlug)
	if err != nil {
		return nil, fmt.Errorf("resolve slug: %w", err)
	}
	sess, err := c.createUploadSession(ctx, fileName, size)
	if err != nil {
		return nil, err
	}
	if err := c.uploadPart(ctx, sess.ID, 1, body); err != nil {
		return nil, err
	}
	if err := c.finishUploadSession(ctx, sess.ID); err != nil {
		return nil, err
	}
	var out attachResults
	path := fmt.Sprintf("/v1/pages/%d/attachments", page.ID)
	if _, err := c.Do(ctx, http.MethodPost, path, attachReq{UploadSessions: []string{sess.ID}}, &out); err != nil {
		return nil, err
	}
	if len(out.Results) == 0 {
		return nil, fmt.Errorf("attach response had no results")
	}
	return &out.Results[0], nil
}

// DeleteAttachment removes an attachment by name from a page. Fails on
// duplicate or missing names with the same messages as DownloadAttachment.
func (c *Client) DeleteAttachment(ctx context.Context, pageSlug, filename string) error {
	page, err := c.GetPage(ctx, pageSlug)
	if err != nil {
		return fmt.Errorf("resolve slug: %w", err)
	}
	atts, err := c.listAttachmentsByID(ctx, page.ID)
	if err != nil {
		return err
	}
	att, err := pickUniqueByName(atts, pageSlug, filename)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/v1/pages/%d/attachments/%d", page.ID, att.ID)
	_, err = c.Do(ctx, http.MethodDelete, path, nil, nil)
	return err
}
