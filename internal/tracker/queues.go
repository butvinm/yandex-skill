package tracker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/butvinm/yandex-skill/internal/render"
)

type Queue struct {
	Key             string  `json:"key"`
	Name            string  `json:"name"`
	Lead            Display `json:"lead"`
	DefaultPriority Display `json:"defaultPriority"`
}

func (q Queue) Plain() string {
	header := q.Key + ": " + q.Name
	meta := render.SkipEmpty(q.Lead.Display, q.DefaultPriority.Display)
	return render.SkipEmptyLines(header, meta)
}

func (q Queue) Row() string {
	return render.SkipEmpty(q.Key, q.Name)
}

func (c *Client) GetQueue(ctx context.Context, key string) (*Queue, error) {
	var out Queue
	_, err := c.Do(ctx, http.MethodGet, "/v3/queues/"+key, nil, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListQueues(ctx context.Context) ([]Queue, error) {
	var all []Queue
	err := c.DoPaginated(ctx, "/v3/queues/", nil, func(raw []byte) error {
		var batch []Queue
		if err := json.Unmarshal(raw, &batch); err != nil {
			return fmt.Errorf("decode queues page: %w", err)
		}
		all = append(all, batch...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return all, nil
}
