package wiki

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCreateUploadSession(t *testing.T) {
	var sent createUploadSessionBody
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/v1/upload_sessions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&sent)
		_, _ = io.WriteString(w, `{"session_id":"u-1","status":"not_started","storage_type":"mds"}`)
	})

	got, err := c.createUploadSession(context.Background(), "ss.png", 12345)
	if err != nil {
		t.Fatal(err)
	}
	if sent.FileName != "ss.png" || sent.FileSize != 12345 {
		t.Errorf("sent = %+v", sent)
	}
	if got.ID != "u-1" || got.Status != "not_started" {
		t.Errorf("got = %+v", got)
	}
}

func TestUploadPart_PutsOctetStream(t *testing.T) {
	var seenCT, seenPart string
	var seenBody []byte
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/v1/upload_sessions/u-1/upload_part" {
			t.Errorf("path = %s", r.URL.Path)
		}
		seenPart = r.URL.Query().Get("part_number")
		seenCT = r.Header.Get("Content-Type")
		seenBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
	})

	if err := c.uploadPart(context.Background(), "u-1", 1, strings.NewReader("payload")); err != nil {
		t.Fatal(err)
	}
	if seenPart != "1" {
		t.Errorf("part_number = %s", seenPart)
	}
	if seenCT != "application/octet-stream" {
		t.Errorf("content-type = %s", seenCT)
	}
	if string(seenBody) != "payload" {
		t.Errorf("body = %q", seenBody)
	}
}

func TestUploadPart_5xx_PreservesStatus(t *testing.T) {
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
		_, _ = io.WriteString(w, `{"detail":"unavailable"}`)
	})
	err := c.uploadPart(context.Background(), "u-1", 1, strings.NewReader("x"))
	if err == nil {
		t.Fatal("want error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 503 {
		t.Errorf("err = %v", err)
	}
}

func TestFinishUploadSession(t *testing.T) {
	var seenBody []byte
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/v1/upload_sessions/u-1/finish" {
			t.Errorf("path = %s", r.URL.Path)
		}
		seenBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
	})
	if err := c.finishUploadSession(context.Background(), "u-1"); err != nil {
		t.Fatal(err)
	}
	if len(seenBody) != 0 {
		t.Errorf("finish should have empty body, got %q", seenBody)
	}
}

func TestUploadSessions_FullRoundTrip(t *testing.T) {
	var got struct {
		create, part, finish int
	}
	c, _ := newWiki(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/upload_sessions":
			got.create++
			_, _ = io.WriteString(w, `{"session_id":"u-1","status":"not_started"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/v1/upload_sessions/u-1/upload_part":
			got.part++
			w.WriteHeader(200)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/upload_sessions/u-1/finish":
			got.finish++
			w.WriteHeader(200)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})

	sess, err := c.createUploadSession(context.Background(), "x", 4)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.uploadPart(context.Background(), sess.ID, 1, strings.NewReader("data")); err != nil {
		t.Fatal(err)
	}
	if err := c.finishUploadSession(context.Background(), sess.ID); err != nil {
		t.Fatal(err)
	}
	if got.create != 1 || got.part != 1 || got.finish != 1 {
		t.Errorf("call counts = %+v", got)
	}
}
