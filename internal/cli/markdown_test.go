package cli

import "testing"

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
