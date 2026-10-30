package internal

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"time"
)

const defaultMaxRetries = 5

func RetryConfig(maxRetries int, singleAttempt bool, initialWait, maxWait time.Duration) (int, time.Duration, time.Duration) {
	if singleAttempt {
		maxRetries = 0
	} else if maxRetries <= 0 {
		maxRetries = defaultMaxRetries
	}
	if initialWait <= 0 {
		initialWait = 2 * time.Second
	}
	if maxWait <= 0 {
		maxWait = 30 * time.Second
	}
	return maxRetries, initialWait, maxWait
}

func WaitBeforeRetry(ctx context.Context, attempt int, initialWait, maxWait time.Duration) error {
	if attempt == 0 {
		return nil
	}
	wait := time.Duration(math.Min(float64(initialWait)*math.Pow(2, float64(attempt-1)), float64(maxWait)))
	select {
	case <-time.After(wait):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func IsRetryableStatus(code int) bool {
	return code == http.StatusTooManyRequests || code == http.StatusServiceUnavailable || code == http.StatusBadGateway || code == http.StatusGatewayTimeout
}

func RetryAfter(resp *http.Response) time.Duration {
	if v := resp.Header.Get("retry-after-ms"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil {
			return time.Duration(ms) * time.Millisecond
		}
	}
	if v := resp.Header.Get("retry-after"); v != "" {
		if sec, err := strconv.Atoi(v); err == nil {
			return time.Duration(sec) * time.Second
		}
	}
	return 0
}
