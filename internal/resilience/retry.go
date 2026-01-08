package resilience

import (
	"fmt"
	"log/slog"
	"time"
)

func Retry(attempts int, delay time.Duration, fn func() error) error {
	var err error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			slog.Info("Retrying request...", "attempt", i+1)
			time.Sleep(delay)
		}

		err = fn()
		if err == nil {
			return nil
		}
	}
	return fmt.Errorf("after %d attempts, last error: %w", attempts, err)
}