package cli

import (
	"path"
	"regexp"
	"strings"
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
