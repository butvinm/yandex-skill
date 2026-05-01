package cli

import (
	"reflect"
	"testing"
)

func TestRewriteServerToLocal(t *testing.T) {
	cases := []struct {
		name    string
		content string
		slug    string
		dir     string
		want    string
	}{
		{
			name:    "yfm image with size hint",
			content: "![изображение.png](/users/m/test/.files/img.png =375x383)",
			slug:    "users/m/test", dir: "att",
			want: "![изображение.png](att/img.png =375x383)",
		},
		{
			name:    "yfm file directive",
			content: `:file[name](/users/m/test/.files/foo.png){type="image/png"}`,
			slug:    "users/m/test", dir: "att",
			want: `:file[name](att/foo.png){type="image/png"}`,
		},
		{
			name:    "legacy 0x0 image",
			content: "0x0:/users/m/test/.files/x.png",
			slug:    "users/m/test", dir: "att",
			want: "0x0:att/x.png",
		},
		{
			name:    "cross-page reference left untouched",
			content: "![foo](/other/page/.files/x.png)",
			slug:    "users/m/test", dir: "att",
			want: "![foo](/other/page/.files/x.png)",
		},
		{
			name:    "multiple matches in one content",
			content: "![a](/s/.files/a.png)\n![b](/s/.files/b.png)",
			slug:    "s", dir: "att",
			want: "![a](att/a.png)\n![b](att/b.png)",
		},
		{
			name:    "transliterated filename with collision suffix",
			content: "![](/users/m/test/.files/izobrazhenie-1.png =256x256)",
			slug:    "users/m/test", dir: "att",
			want: "![](att/izobrazhenie-1.png =256x256)",
		},
		{
			name: "empty content", content: "",
			slug: "users/m/test", dir: "att",
			want: "",
		},
		{
			name:    "no matches in plain prose",
			content: "hello world\nthis is markdown",
			slug:    "users/m/test", dir: "att",
			want: "hello world\nthis is markdown",
		},
		{
			name:    "slug with regex metacharacters is escaped",
			content: "![](/a.b/c+d/.files/foo.png)",
			slug:    "a.b/c+d", dir: "att",
			want: "![](att/foo.png)",
		},
		{
			name:    "trailing slash in attachmentsDir is normalized",
			content: "![](/s/.files/x.png)",
			slug:    "s", dir: "att/",
			want: "![](att/x.png)",
		},
		{
			name:    "absolute attachmentsDir preserved",
			content: "![](/s/.files/x.png)",
			slug:    "s", dir: "/tmp/att",
			want: "![](/tmp/att/x.png)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rewriteServerToLocal(tc.content, tc.slug, tc.dir)
			if got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestFindLocalAttachmentRefs(t *testing.T) {
	cases := []struct {
		name    string
		content string
		dir     string
		want    []string
	}{
		{
			name:    "image syntax",
			content: "![alt](att/foo.png)",
			dir:     "att",
			want:    []string{"foo.png"},
		},
		{
			name:    "file directive",
			content: `:file[name](att/doc.pdf){type="application/pdf"}`,
			dir:     "att",
			want:    []string{"doc.pdf"},
		},
		{
			name:    "mixed image and directive",
			content: "![](att/a.png) text :file[](att/b.pdf){type=\"x\"}",
			dir:     "att",
			want:    []string{"a.png", "b.pdf"},
		},
		{
			name:    "duplicates collapsed to unique",
			content: "![](att/x.png) ![](att/x.png) ![](att/y.png)",
			dir:     "att",
			want:    []string{"x.png", "y.png"},
		},
		{
			name:    "no matches",
			content: "plain text with no references",
			dir:     "att",
			want:    []string{},
		},
		{
			name: "empty content", content: "",
			dir:  "att",
			want: []string{},
		},
		{
			name:    "trailing slash in dir is normalized",
			content: "![](att/x.png)",
			dir:     "att/",
			want:    []string{"x.png"},
		},
		{
			name:    "absolute dir",
			content: "![](/tmp/att/x.png)",
			dir:     "/tmp/att",
			want:    []string{"x.png"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := findLocalAttachmentRefs(tc.content, tc.dir)
			// Treat nil and empty-slice as equal for empty-want cases.
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got  %v\nwant %v", got, tc.want)
			}
		})
	}
}

func TestRewriteLocalToServer(t *testing.T) {
	cases := []struct {
		name    string
		content string
		dir     string
		urls    map[string]string
		want    string
	}{
		{
			name:    "image rewritten when basename mapped",
			content: "![](att/foo.png)",
			dir:     "att",
			urls:    map[string]string{"foo.png": "/users/m/test/.files/foo.png"},
			want:    "![](/users/m/test/.files/foo.png)",
		},
		{
			name:    "file directive rewritten",
			content: `:file[name](att/doc.pdf){type="application/pdf"}`,
			dir:     "att",
			urls:    map[string]string{"doc.pdf": "/users/m/test/.files/doc.pdf"},
			want:    `:file[name](/users/m/test/.files/doc.pdf){type="application/pdf"}`,
		},
		{
			name:    "missing basename left untouched",
			content: "![](att/foo.png) ![](att/bar.png)",
			dir:     "att",
			urls:    map[string]string{"foo.png": "/s/.files/foo.png"},
			want:    "![](/s/.files/foo.png) ![](att/bar.png)",
		},
		{
			name:    "empty url map is a no-op",
			content: "![](att/foo.png)",
			dir:     "att",
			urls:    map[string]string{},
			want:    "![](att/foo.png)",
		},
		{
			name:    "multiple basenames all rewritten",
			content: "![](att/a.png)\n![](att/b.png)",
			dir:     "att",
			urls: map[string]string{
				"a.png": "/s/.files/a.png",
				"b.png": "/s/.files/b.png",
			},
			want: "![](/s/.files/a.png)\n![](/s/.files/b.png)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rewriteLocalToServer(tc.content, tc.dir, tc.urls)
			if got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}
