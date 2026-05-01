package wiki

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestAttachment_Plain(t *testing.T) {
	a := Attachment{Name: "ss.png", Size: 2048, Mimetype: "image/png", CreatedAt: "2026-05-01"}
	got := a.Plain()
	want := "ss.png  2KB  image/png  2026-05-01"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestAttachment_Row_SkipsEmptyMime(t *testing.T) {
	a := Attachment{Name: "x", Size: 0, CreatedAt: "2026-05-01"}
	got := a.Row()
	want := "x  0B  2026-05-01"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{
		0:                   "0B",
		512:                 "512B",
		1024:                "1KB",
		1536:                "1KB",
		1024 * 1024:         "1.0MB",
		3 * 1024 * 1024 / 2: "1.5MB",
		1024 * 1024 * 1024:  "1.0GB",
	}
	for n, want := range cases {
		if got := humanSize(n); got != want {
			t.Errorf("humanSize(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestListAttachments_ResolvesSlugThenPaginates(t *testing.T) {
	calls := 0
	var lastCursor string
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/pages":
			if r.URL.Query().Get("slug") != "team/notes" {
				t.Errorf("slug = %s", r.URL.Query().Get("slug"))
			}
			_, _ = io.WriteString(w, `{"id":42,"slug":"team/notes","title":"T"}`)
		case r.URL.Path == "/v1/pages/42/attachments":
			calls++
			lastCursor = r.URL.Query().Get("cursor")
			if r.URL.Query().Get("page_size") != "100" {
				t.Errorf("page_size = %s", r.URL.Query().Get("page_size"))
			}
			if calls == 1 {
				_, _ = io.WriteString(w, `{"results":[{"id":1,"name":"a.png","size":10,"mimetype":"image/png","check_status":"ready"}],"next_cursor":"c2"}`)
				return
			}
			_, _ = io.WriteString(w, `{"results":[{"id":2,"name":"b.pdf","size":20,"check_status":"ready"}]}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})

	got, err := c.ListAttachments(context.Background(), "team/notes")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("attachment calls = %d", calls)
	}
	if lastCursor != "c2" {
		t.Errorf("second call cursor = %q", lastCursor)
	}
	if len(got) != 2 || got[0].Name != "a.png" || got[1].Name != "b.pdf" {
		t.Errorf("got = %+v", got)
	}
}

func TestDownloadAttachment_HappyPath(t *testing.T) {
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/pages":
			_, _ = io.WriteString(w, `{"id":42,"slug":"team/notes","title":"T"}`)
		case "/v1/pages/42/attachments":
			_, _ = io.WriteString(w, `{"results":[{"id":1,"name":"diagram.png","check_status":"ready"}]}`)
		case "/v1/pages/attachments/download_by_url":
			if r.URL.Query().Get("url") != "team/notes/diagram.png" {
				t.Errorf("url = %s", r.URL.Query().Get("url"))
			}
			if r.URL.Query().Get("download") != "true" {
				t.Errorf("download = %s", r.URL.Query().Get("download"))
			}
			_, _ = w.Write([]byte("\x89PNG\x0d\x0a"))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})

	var buf bytes.Buffer
	if err := c.DownloadAttachment(context.Background(), "team/notes", "diagram.png", &buf); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "\x89PNG\x0d\x0a" {
		t.Errorf("body = %q", buf.String())
	}
}

func TestDownloadAttachment_NotReady(t *testing.T) {
	called := false
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/pages":
			_, _ = io.WriteString(w, `{"id":42,"slug":"team/notes","title":"T"}`)
		case "/v1/pages/42/attachments":
			_, _ = io.WriteString(w, `{"results":[{"id":1,"name":"infected.exe","check_status":"infected"}]}`)
		case "/v1/pages/attachments/download_by_url":
			called = true
			w.WriteHeader(500)
		}
	})

	var buf bytes.Buffer
	err := c.DownloadAttachment(context.Background(), "team/notes", "infected.exe", &buf)
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "infected") {
		t.Errorf("err = %v", err)
	}
	if called {
		t.Error("download endpoint should not have been hit")
	}
	if buf.Len() != 0 {
		t.Error("buf should be empty on refusal")
	}
}

func TestDownloadAttachment_DuplicateNames(t *testing.T) {
	called := false
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/pages":
			_, _ = io.WriteString(w, `{"id":42,"slug":"team/notes","title":"T"}`)
		case "/v1/pages/42/attachments":
			_, _ = io.WriteString(w, `{"results":[{"id":1,"name":"x","check_status":"ready"},{"id":2,"name":"x","check_status":"ready"}]}`)
		case "/v1/pages/attachments/download_by_url":
			called = true
		}
	})

	err := c.DownloadAttachment(context.Background(), "team/notes", "x", io.Discard)
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "multiple attachments named") {
		t.Errorf("err = %v", err)
	}
	if called {
		t.Error("download must not run on ambiguous match")
	}
}

func TestDownloadAttachment_NotFound(t *testing.T) {
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/pages":
			_, _ = io.WriteString(w, `{"id":42,"slug":"team/notes","title":"T"}`)
		case "/v1/pages/42/attachments":
			_, _ = io.WriteString(w, `{"results":[]}`)
		}
	})
	err := c.DownloadAttachment(context.Background(), "team/notes", "missing.png", io.Discard)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v", err)
	}
}

func TestListAttachments_ResolveFails(t *testing.T) {
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = io.WriteString(w, `{"detail":"not found"}`)
	})
	_, err := c.ListAttachments(context.Background(), "missing")
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "resolve slug") {
		t.Errorf("err = %v", err)
	}
}
