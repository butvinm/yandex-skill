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
