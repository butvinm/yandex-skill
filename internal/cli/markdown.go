package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/butvinm/yandex-skill/internal/wiki"
)

// attachmentURLRegex matches every `/<pageSlug>/.files/<filename>` URL in
// page content. The terminator class stops at common markdown surroundings
// (whitespace, `)`, `]`, `"`, `'`, `}`) so the URL doesn't over-match into
// adjacent syntax. Scoped to the given slug — cross-page references like
// `/<other-slug>/.files/X` are intentionally untouched.
func attachmentURLRegex(pageSlug string) *regexp.Regexp {
	return regexp.MustCompile("/" + regexp.QuoteMeta(pageSlug) + `/\.files/[^\s)\]"'}]+`)
}

// rewriteServerToLocal replaces every page-scoped attachment URL in content
// with a local relative path under attachmentsDir. The local filename is
// path.Base(serverURL) — the server-canonical name, which can be mangled
// (transliteration) or suffix-disambiguated (`-1`, `-2`) relative to the
// Attachment.Name field. Use the URL basename for stable round-tripping.
//
// attachmentsDir's trailing slash is normalized; otherwise it is taken
// verbatim (relative or absolute, both fine).
func rewriteServerToLocal(content, pageSlug, attachmentsDir string) string {
	dir := strings.TrimRight(attachmentsDir, "/")
	re := attachmentURLRegex(pageSlug)
	return re.ReplaceAllStringFunc(content, func(match string) string {
		return dir + "/" + path.Base(match)
	})
}

// localRefRegex matches `<attachmentsDir>/<filename>` substrings in local
// content. Mirrors attachmentURLRegex's terminator class.
//
// Caveat: this is a dumb substring matcher — if attachmentsDir happens to
// occur as a tail of an unrelated path in the content (e.g. `foo/att/bar`
// when dir is `att`), it will false-positive match. In practice users pass
// disambiguating dirs like `./att` or absolute paths, so this hasn't bitten
// us. Tighten only if it does.
func localRefRegex(attachmentsDir string) *regexp.Regexp {
	dir := strings.TrimRight(attachmentsDir, "/")
	return regexp.MustCompile(regexp.QuoteMeta(dir) + `/[^\s)\]"'}]+`)
}

// findLocalAttachmentRefs returns the unique set of basenames referenced as
// `<attachmentsDir>/<basename>` in content. Used by the upload-side flow to
// decide which local files to upload. Order matches first appearance.
func findLocalAttachmentRefs(content, attachmentsDir string) []string {
	re := localRefRegex(attachmentsDir)
	matches := re.FindAllString(content, -1)
	seen := make(map[string]struct{}, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		b := path.Base(m)
		if _, ok := seen[b]; ok {
			continue
		}
		seen[b] = struct{}{}
		out = append(out, b)
	}
	return out
}

// rewriteLocalToServer replaces every `<attachmentsDir>/<basename>` substring
// in content with urlByBasename[basename]. Basenames missing from the map
// are left unchanged — caller decides whether that's an error (usually it
// just means the local file wasn't uploaded and the reference stays local).
func rewriteLocalToServer(content, attachmentsDir string, urlByBasename map[string]string) string {
	re := localRefRegex(attachmentsDir)
	return re.ReplaceAllStringFunc(content, func(match string) string {
		b := path.Base(match)
		if url, ok := urlByBasename[b]; ok {
			return url
		}
		return match
	})
}

// refuseGrid returns an error for grid pages, which store structured table
// data at /v1/grids/{id} and have no markdown content. Other page types
// (wysiwyg, legacy page, unknown) pass through.
func refuseGrid(pageType string) error {
	if pageType == wiki.PageTypeGrid {
		return errors.New("page_type=grid: structured table, not markdown content (see /v1/grids/{id})")
	}
	return nil
}

// warnNonWysiwyg emits a stderr warning when the page is not modern YFM
// markdown. Grid pages should be filtered out via refuseGrid first; this
// only warns for legacy `page` and unknown types.
func warnNonWysiwyg(pageType string, stderr io.Writer) {
	if pageType == wiki.PageTypeWysiwyg || pageType == wiki.PageTypeGrid {
		return
	}
	fmt.Fprintf(stderr, "warning: page_type=%q: content may not be Yandex Flavored Markdown; attachment-link rewriting may have no effect\n", pageType)
}

// syncAttachmentsForGet downloads every attachment on the page into
// attachmentsDir and returns the page content with server attachment URLs
// rewritten to local relative paths.
//
// Refuses grid pages outright. Warns on other non-wysiwyg types but
// proceeds — the rewrite is a no-op when content has no `/<slug>/.files/X`
// matches, which is the common case for plain legacy pages.
//
// Downloads every attachment, not only the ones referenced inline:
// attachments can live in the page sidebar without an inline link, and
// dropping them on get would silently lose data on round-trip.
func syncAttachmentsForGet(ctx context.Context, client *wiki.Client, page *wiki.Page, attachmentsDir string, stderr io.Writer) (string, error) {
	if err := refuseGrid(page.PageType); err != nil {
		return "", err
	}
	warnNonWysiwyg(page.PageType, stderr)
	if err := os.MkdirAll(attachmentsDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir attachments-dir: %w", err)
	}
	atts, err := client.ListAttachments(ctx, page.Slug)
	if err != nil {
		return "", fmt.Errorf("list attachments: %w", err)
	}
	for _, att := range atts {
		if att.CheckStatus != "" && att.CheckStatus != "ready" {
			return "", fmt.Errorf("attachment %q has check_status=%s; refusing to download", att.Name, att.CheckStatus)
		}
		urlName := path.Base(att.DownloadURL)
		dst := filepath.Join(attachmentsDir, urlName)
		f, err := os.Create(dst)
		if err != nil {
			return "", fmt.Errorf("create %s: %w", dst, err)
		}
		if err := client.DownloadAttachmentByURL(ctx, att.DownloadURL, f); err != nil {
			_ = f.Close()
			return "", fmt.Errorf("download %s: %w", urlName, err)
		}
		if err := f.Close(); err != nil {
			return "", fmt.Errorf("close %s: %w", dst, err)
		}
	}
	return rewriteServerToLocal(page.Content, page.Slug, attachmentsDir), nil
}

// syncAttachmentsForWrite uploads local attachment files referenced from
// localContent that aren't already present on the page, then returns the
// content with `<attachmentsDir>/<X>` references rewritten to the server-
// returned download URLs.
//
// Match-by-basename: a local `att/foo.png` is considered "already on the
// server" iff there is an existing attachment whose `path.Base(download_url)
// == "foo.png"`. The byte content isn't compared (the API doesn't surface
// a checksum), so re-uploading after editing the local file requires the
// user to delete the server attachment first via `wiki attachments delete`.
//
// Refuses grid pages outright; warns on legacy `page` and unknown types.
// Does NOT delete server attachments that aren't referenced locally —
// one-directional drift may be intentional.
func syncAttachmentsForWrite(ctx context.Context, client *wiki.Client, page *wiki.Page, localContent, attachmentsDir string, stderr io.Writer) (string, error) {
	if err := refuseGrid(page.PageType); err != nil {
		return "", err
	}
	warnNonWysiwyg(page.PageType, stderr)
	basenames := findLocalAttachmentRefs(localContent, attachmentsDir)
	if len(basenames) == 0 {
		// No local refs → nothing to upload, rewrite is a no-op.
		return localContent, nil
	}
	atts, err := client.ListAttachments(ctx, page.Slug)
	if err != nil {
		return "", fmt.Errorf("list attachments: %w", err)
	}
	serverURLByBasename := make(map[string]string, len(atts))
	for _, a := range atts {
		serverURLByBasename[path.Base(a.DownloadURL)] = a.DownloadURL
	}
	urlByBasename := make(map[string]string, len(basenames))
	for _, b := range basenames {
		if url, ok := serverURLByBasename[b]; ok {
			urlByBasename[b] = url
			continue
		}
		src := filepath.Join(attachmentsDir, b)
		uploaded, err := uploadLocalFile(ctx, client, page.Slug, src, b)
		if err != nil {
			return "", fmt.Errorf("upload %s: %w", b, err)
		}
		urlByBasename[b] = uploaded.DownloadURL
	}
	return rewriteLocalToServer(localContent, attachmentsDir, urlByBasename), nil
}

// uploadLocalFile is a thin wrapper around wiki.Client.UploadAttachment that
// handles the file open/stat/close boilerplate.
func uploadLocalFile(ctx context.Context, client *wiki.Client, slug, src, name string) (*wiki.Attachment, error) {
	f, err := os.Open(src)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	return client.UploadAttachment(ctx, slug, name, f, info.Size())
}
