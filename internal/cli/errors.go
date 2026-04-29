package cli

import (
	"errors"

	"github.com/butvinm/yandex-cli/internal/tracker"
	"github.com/butvinm/yandex-cli/internal/wiki"
)

func statusFromErr(err error) int {
	var t *tracker.APIError
	if errors.As(err, &t) {
		return t.Status
	}
	var w *wiki.APIError
	if errors.As(err, &w) {
		return w.Status
	}
	return 0
}
