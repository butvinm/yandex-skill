package wiki

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// uploadSession is the wire shape returned by /v1/upload_sessions endpoints.
type uploadSession struct {
	ID          string `json:"session_id"`
	Status      string `json:"status"`
	StorageType string `json:"storage_type"`
}

type createUploadSessionBody struct {
	FileName string `json:"file_name"`
	FileSize int64  `json:"file_size"`
}

func (c *Client) createUploadSession(ctx context.Context, fileName string, fileSize int64) (*uploadSession, error) {
	body := createUploadSessionBody{FileName: fileName, FileSize: fileSize}
	var out uploadSession
	if _, err := c.Do(ctx, http.MethodPost, "/v1/upload_sessions", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) uploadPart(ctx context.Context, sessionID string, partNumber int, body io.Reader) error {
	path := fmt.Sprintf("/v1/upload_sessions/%s/upload_part?part_number=%d", sessionID, partNumber)
	resp, err := c.DoRaw(ctx, http.MethodPut, path, "application/octet-stream", body)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) finishUploadSession(ctx context.Context, sessionID string) error {
	path := fmt.Sprintf("/v1/upload_sessions/%s/finish", sessionID)
	_, err := c.Do(ctx, http.MethodPost, path, nil, nil)
	return err
}
