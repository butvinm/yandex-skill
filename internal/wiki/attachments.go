package wiki

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/butvinm/yandex-skill/internal/render"
)

// MaxAttachmentSize caps single-part uploads. The Yandex Wiki upload-sessions
// API accepts parts of 5..16 MiB; we ship single-part only to keep the client
// lean, so we hard-cap at 16 MiB.
const MaxAttachmentSize = 16 * 1024 * 1024

type Attachment struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// Size is the API's reported size as a free-form string. Yandex Wiki
	// returns it quoted in JSON (e.g. "0.10") with units that aren't
	// documented and small files truncate to "0.00", so the value is
	// passed through verbatim and not surfaced in plain output.
	Size        string `json:"size"`
	Mimetype    string `json:"mimetype"`
	DownloadURL string `json:"download_url"`
	CreatedAt   string `json:"created_at"`
	CheckStatus string `json:"check_status"`
	HasPreview  bool   `json:"has_preview"`
}

// Plain emits "name  mime  created_at  download_url" with two-space
// separators. download_url trails because it's the longest field; agents
// embedding into markdown can extract it with `awk -F'  ' '{print $NF}'`.
// Empty fields (e.g. unknown mime) are skipped via render.SkipEmpty.
func (a Attachment) Plain() string {
	return render.SkipEmpty(a.Name, a.Mimetype, a.CreatedAt, a.DownloadURL)
}

func (a Attachment) Row() string { return a.Plain() }

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

// pickAttachment picks the single attachment matching ident on the page.
// ident is matched against the attachment's `name` first; if that's
// ambiguous it falls back to the last segment of `download_url`, which
// the server makes unique by suffixing duplicates (`foo.png`,
// `foo-1.png`, `foo-2.png`...). On unresolvable ambiguity the error
// lists the URL-filename forms so the caller can re-run with one.
func pickAttachment(atts []Attachment, pageSlug, ident string) (*Attachment, error) {
	var byName, byURLName []*Attachment
	for i := range atts {
		a := &atts[i]
		if a.Name == ident {
			byName = append(byName, a)
		}
		if path.Base(a.DownloadURL) == ident {
			byURLName = append(byURLName, a)
		}
	}
	switch len(byName) {
	case 1:
		return byName[0], nil
	case 0:
		if len(byURLName) == 1 {
			return byURLName[0], nil
		}
		return nil, fmt.Errorf("attachment %q not found on page %q", ident, pageSlug)
	default:
		hints := make([]string, 0, len(byName))
		for _, a := range byName {
			hints = append(hints, path.Base(a.DownloadURL))
		}
		return nil, fmt.Errorf("multiple attachments named %q on %q; pass one of these URL filenames instead: %s", ident, pageSlug, strings.Join(hints, ", "))
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
	att, err := pickAttachment(atts, pageSlug, filename)
	if err != nil {
		return err
	}
	if att.CheckStatus != "" && att.CheckStatus != "ready" {
		return fmt.Errorf("attachment %q has check_status=%s; refusing to download", filename, att.CheckStatus)
	}
	return c.DownloadAttachmentByURL(ctx, att.DownloadURL, w)
}

// DownloadAttachmentByURL streams the attachment at downloadURL to w. Use
// this when you already have the URL from a prior ListAttachments call —
// it skips the slug+name resolution (and its ambiguity errors) that
// DownloadAttachment performs.
func (c *Client) DownloadAttachmentByURL(ctx context.Context, downloadURL string, w io.Writer) error {
	q := url.Values{}
	q.Set("url", strings.TrimPrefix(downloadURL, "/"))
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
	att, err := pickAttachment(atts, pageSlug, filename)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/v1/pages/%d/attachments/%d", page.ID, att.ID)
	_, err = c.Do(ctx, http.MethodDelete, path, nil, nil)
	return err
}
