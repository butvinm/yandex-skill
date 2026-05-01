package wiki

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestAttachment_Plain(t *testing.T) {
	a := Attachment{
		Name:        "ss.png",
		Size:        "0.10",
		Mimetype:    "image/png",
		CreatedAt:   "2026-05-01",
		DownloadURL: "https://api.wiki.yandex.net/v1/.../download/abc",
	}
	got := a.Plain()
	want := "ss.png  image/png  2026-05-01  https://api.wiki.yandex.net/v1/.../download/abc"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestAttachment_Row_SkipsEmptyMimeAndURL(t *testing.T) {
	a := Attachment{Name: "x", CreatedAt: "2026-05-01"}
	got := a.Row()
	want := "x  2026-05-01"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// Yandex Wiki returns size as a quoted decimal string ("0.00", "0.10")
// rather than a JSON number. Decoding into an int64 used to fail on every
// real attachment; pin the wire shape here so we don't regress.
func TestAttachment_DecodesAPIShape(t *testing.T) {
	body := `{"id":12465011,"name":"docker-compose_keycloak.yml","size":"0.00","mimetype":"application/octet-stream","download_url":"/documentation/kekloack/.files/docker-composekeycloak.yml","created_at":"2022-09-09T12:57:23.323Z","check_status":"ready","has_preview":false}`
	var a Attachment
	if err := json.Unmarshal([]byte(body), &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if a.Size != "0.00" {
		t.Errorf("Size = %q, want %q", a.Size, "0.00")
	}
	if a.Name != "docker-compose_keycloak.yml" {
		t.Errorf("Name = %q", a.Name)
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
				_, _ = io.WriteString(w, `{"results":[{"id":1,"name":"a.png","size":"10","mimetype":"image/png","check_status":"ready"}],"next_cursor":"c2"}`)
				return
			}
			_, _ = io.WriteString(w, `{"results":[{"id":2,"name":"b.pdf","size":"20","check_status":"ready"}]}`)
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
			_, _ = io.WriteString(w, `{"results":[{"id":1,"name":"diagram.png","download_url":"/team/notes/.files/diagram.png","check_status":"ready"}]}`)
		case "/v1/pages/attachments/download_by_url":
			if r.URL.Query().Get("url") != "team/notes/.files/diagram.png" {
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

// The server's download_by_url endpoint expects the URL form returned in
// the attachment's download_url field — typically with a server-mangled
// filename (e.g. underscores stripped, transliterated). Constructing the
// query as `<slug>/<name>` failed with HTTP 400 on real attachments.
func TestDownloadAttachment_UsesDownloadURLNotName(t *testing.T) {
	var sentURL string
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/pages":
			_, _ = io.WriteString(w, `{"id":42,"slug":"team/notes","title":"T"}`)
		case "/v1/pages/42/attachments":
			_, _ = io.WriteString(w, `{"results":[{"id":1,"name":"docker-compose_keycloak.yml","download_url":"/team/notes/.files/docker-composekeycloak.yml","check_status":"ready"}]}`)
		case "/v1/pages/attachments/download_by_url":
			sentURL = r.URL.Query().Get("url")
			_, _ = w.Write([]byte("YAMLDATA"))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})

	var buf bytes.Buffer
	if err := c.DownloadAttachment(context.Background(), "team/notes", "docker-compose_keycloak.yml", &buf); err != nil {
		t.Fatal(err)
	}
	if sentURL != "team/notes/.files/docker-composekeycloak.yml" {
		t.Errorf("download_by_url url = %q, want server-mangled form from download_url field", sentURL)
	}
	if buf.String() != "YAMLDATA" {
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

func TestDownloadAttachment_DuplicateNames_ErrorListsURLFilenames(t *testing.T) {
	called := false
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/pages":
			_, _ = io.WriteString(w, `{"id":42,"slug":"team/notes","title":"T"}`)
		case "/v1/pages/42/attachments":
			_, _ = io.WriteString(w, `{"results":[{"id":1,"name":"image.png","download_url":"/team/notes/.files/image.png","check_status":"ready"},{"id":2,"name":"image.png","download_url":"/team/notes/.files/image-1.png","check_status":"ready"}]}`)
		case "/v1/pages/attachments/download_by_url":
			called = true
		}
	})

	err := c.DownloadAttachment(context.Background(), "team/notes", "image.png", io.Discard)
	if err == nil {
		t.Fatal("want error")
	}
	for _, want := range []string{"multiple attachments named", "image.png", "image-1.png"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err missing %q: %v", want, err)
		}
	}
	if called {
		t.Error("download must not run on ambiguous match")
	}
}

// Yandex Wiki accepts duplicate `name` values, so the original error-out
// behavior locked users out of pages with same-named attachments. Pin the
// fallback: passing the URL filename (server-unique) resolves the ambiguity.
func TestDownloadAttachment_ResolvesByURLFilename(t *testing.T) {
	var sentURL string
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/pages":
			_, _ = io.WriteString(w, `{"id":42,"slug":"team/notes","title":"T"}`)
		case "/v1/pages/42/attachments":
			_, _ = io.WriteString(w, `{"results":[{"id":1,"name":"image.png","download_url":"/team/notes/.files/image.png","check_status":"ready"},{"id":2,"name":"image.png","download_url":"/team/notes/.files/image-1.png","check_status":"ready"}]}`)
		case "/v1/pages/attachments/download_by_url":
			sentURL = r.URL.Query().Get("url")
			_, _ = w.Write([]byte("PNG2"))
		}
	})

	var buf bytes.Buffer
	if err := c.DownloadAttachment(context.Background(), "team/notes", "image-1.png", &buf); err != nil {
		t.Fatal(err)
	}
	if sentURL != "team/notes/.files/image-1.png" {
		t.Errorf("download_by_url url = %q, want second attachment", sentURL)
	}
	if buf.String() != "PNG2" {
		t.Errorf("body = %q", buf.String())
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

func TestUploadAttachment_FullFlow(t *testing.T) {
	var calls []string
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		calls = append(calls, key)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pages":
			_, _ = io.WriteString(w, `{"id":42,"slug":"team/notes","title":"T"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/upload_sessions":
			_, _ = io.WriteString(w, `{"session_id":"u-1","status":"not_started"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/v1/upload_sessions/u-1/upload_part":
			body, _ := io.ReadAll(r.Body)
			if string(body) != "PNGDATA" {
				t.Errorf("part body = %q", body)
			}
			w.WriteHeader(200)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/upload_sessions/u-1/finish":
			w.WriteHeader(200)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/pages/42/attachments":
			var sent attachReq
			_ = json.NewDecoder(r.Body).Decode(&sent)
			if len(sent.UploadSessions) != 1 || sent.UploadSessions[0] != "u-1" {
				t.Errorf("attach req = %+v", sent)
			}
			_, _ = io.WriteString(w, `{"results":[{"id":7,"name":"diagram.png","size":"7","check_status":"ready"}]}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})

	got, err := c.UploadAttachment(context.Background(), "team/notes", "diagram.png", strings.NewReader("PNGDATA"), 7)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != 7 || got.Name != "diagram.png" {
		t.Errorf("got = %+v", got)
	}
	wantOrder := []string{
		"GET /v1/pages",
		"POST /v1/upload_sessions",
		"PUT /v1/upload_sessions/u-1/upload_part",
		"POST /v1/upload_sessions/u-1/finish",
		"POST /v1/pages/42/attachments",
	}
	if len(calls) != len(wantOrder) {
		t.Fatalf("calls = %v", calls)
	}
	for i, w := range wantOrder {
		if calls[i] != w {
			t.Errorf("call[%d] = %s, want %s", i, calls[i], w)
		}
	}
}

func TestUploadAttachment_RejectsOversize(t *testing.T) {
	called := false
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	_, err := c.UploadAttachment(context.Background(), "team/notes", "big.bin", strings.NewReader(""), MaxAttachmentSize+1)
	if err == nil || !strings.Contains(err.Error(), "file too large") {
		t.Fatalf("err = %v", err)
	}
	if called {
		t.Error("no HTTP call should be made when size exceeds cap")
	}
}

func TestDeleteAttachment_HappyPath(t *testing.T) {
	deleteCalled := false
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pages":
			_, _ = io.WriteString(w, `{"id":42,"slug":"team/notes","title":"T"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pages/42/attachments":
			_, _ = io.WriteString(w, `{"results":[{"id":7,"name":"x.png"}]}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/pages/42/attachments/7":
			deleteCalled = true
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})

	if err := c.DeleteAttachment(context.Background(), "team/notes", "x.png"); err != nil {
		t.Fatal(err)
	}
	if !deleteCalled {
		t.Error("DELETE not issued")
	}
}

func TestDeleteAttachment_NotFound(t *testing.T) {
	deleteCalled := false
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pages":
			_, _ = io.WriteString(w, `{"id":42,"slug":"team/notes","title":"T"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pages/42/attachments":
			_, _ = io.WriteString(w, `{"results":[]}`)
		case r.Method == http.MethodDelete:
			deleteCalled = true
		}
	})
	err := c.DeleteAttachment(context.Background(), "team/notes", "missing")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v", err)
	}
	if deleteCalled {
		t.Error("DELETE should not run on miss")
	}
}

func TestDeleteAttachment_DuplicateNames_ErrorListsURLFilenames(t *testing.T) {
	deleteCalled := false
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pages":
			_, _ = io.WriteString(w, `{"id":42,"slug":"team/notes","title":"T"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pages/42/attachments":
			_, _ = io.WriteString(w, `{"results":[{"id":1,"name":"x","download_url":"/team/notes/.files/x.bin"},{"id":2,"name":"x","download_url":"/team/notes/.files/x-1.bin"}]}`)
		case r.Method == http.MethodDelete:
			deleteCalled = true
		}
	})
	err := c.DeleteAttachment(context.Background(), "team/notes", "x")
	if err == nil {
		t.Fatal("want error")
	}
	for _, want := range []string{"multiple attachments named", "x.bin", "x-1.bin"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err missing %q: %v", want, err)
		}
	}
	if deleteCalled {
		t.Error("DELETE must not run on ambiguity")
	}
}

func TestDeleteAttachment_ResolvesByURLFilename(t *testing.T) {
	deletedID := ""
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pages":
			_, _ = io.WriteString(w, `{"id":42,"slug":"team/notes","title":"T"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pages/42/attachments":
			_, _ = io.WriteString(w, `{"results":[{"id":1,"name":"x","download_url":"/team/notes/.files/x.bin"},{"id":2,"name":"x","download_url":"/team/notes/.files/x-1.bin"}]}`)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v1/pages/42/attachments/"):
			deletedID = strings.TrimPrefix(r.URL.Path, "/v1/pages/42/attachments/")
			w.WriteHeader(204)
		}
	})
	if err := c.DeleteAttachment(context.Background(), "team/notes", "x-1.bin"); err != nil {
		t.Fatal(err)
	}
	if deletedID != "2" {
		t.Errorf("deleted id = %q, want 2 (the row whose URL filename was passed)", deletedID)
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
