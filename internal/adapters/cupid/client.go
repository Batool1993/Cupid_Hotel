// internal/adapters/cupid/client.go
package cupid

import (
	"context"
	crand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

type Client struct {
	base string
	hc   *http.Client
	key  string
	rl   *rate.Limiter
}

func New(base, key string, rps int) (*Client, error) {
	if key == "" {
		return nil, fmt.Errorf("API key is required")
	}
	if rps <= 0 {
		rps = 5
	}
	return &Client{
		base: base,
		hc:   &http.Client{Timeout: 20 * time.Second},
		key:  key,
		rl:   rate.NewLimiter(rate.Limit(rps), rps),
	}, nil
}

// ---- Public API (tries modern endpoints first, falls back to legacy variants) ----

func (c *Client) GetProperty(ctx context.Context, id int64) (map[string]any, error) {
	candidates := []string{
		fmt.Sprintf("%s/properties/%d", c.base, id), // preferred
		fmt.Sprintf("%s/property/%d", c.base, id),   // legacy
	}
	var out map[string]any
	return out, c.getFirst(ctx, candidates, &out)
}

func (c *Client) GetTranslation(ctx context.Context, id int64, lang string) (map[string]any, error) {
	candidates := []string{
		fmt.Sprintf("%s/properties/%d/translations/%s", c.base, id, lang), // preferred
		fmt.Sprintf("%s/properties/%d/translation/%s", c.base, id, lang),
		fmt.Sprintf("%s/properties/%d/lang/%s", c.base, id, lang),
		fmt.Sprintf("%s/property/%d/lang/%s", c.base, id, lang), // legacy
	}
	var out map[string]any
	return out, c.getFirst(ctx, candidates, &out)
}

func (c *Client) GetReviews(ctx context.Context, id int64, count int) ([]map[string]any, error) {
	candidates := []string{
		fmt.Sprintf("%s/properties/%d/reviews?limit=%d", c.base, id, count), // preferred
		fmt.Sprintf("%s/properties/%d/reviews/%d", c.base, id, count),
		fmt.Sprintf("%s/property/reviews/%d/%d", c.base, id, count), // legacy
	}
	var out []map[string]any
	return out, c.getFirst(ctx, candidates, &out)
}

// ---- Internals ----

var (
	ErrNotFound     = errors.New("cupid: not found")
	ErrUnauthorized = errors.New("cupid: unauthorized")
	ErrForbidden    = errors.New("cupid: forbidden")
)

func (c *Client) getFirst(ctx context.Context, urls []string, out any) error {
	var last error
	for _, u := range urls {
		if err := c.get(ctx, u, out); err != nil {
			if errors.Is(err, ErrNotFound) {
				last = err
				continue // try next pattern
			}
			return err // non-404: stop early
		}
		return nil // success
	}
	if last != nil {
		return last
	}
	return errors.New("no candidate URL succeeded")
}

// get performs a GET with client-side rate limiting, retries, and JSON decode into out.
// Retries on 429 and transient 5xx, honoring Retry-After when provided.
func (c *Client) get(ctx context.Context, url string, out any) error {
	// client-side rate limiting
	if err := c.rl.Wait(ctx); err != nil {
		return err
	}

	var lastErr error
	for i := 0; i < 4; i++ {
		// build a fresh request each attempt
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		if c.key != "" {
			req.Header.Set("X-API-Key", c.key)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "cupid-hotel/1.0")

		resp, err := c.hc.Do(req)
		if err != nil {
			// network error or context canceled
			if ctx.Err() != nil {
				return ctx.Err()
			}
			lastErr = err
			// context-aware sleep before retry
			if i < 3 && sleepCtx(ctx, backoff(i)) {
				continue
			}
			// no more retries or context canceled
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return lastErr
		}

		switch resp.StatusCode {
		case http.StatusOK, http.StatusCreated, http.StatusAccepted:
			// decode then close
			err := json.NewDecoder(resp.Body).Decode(out)
			resp.Body.Close()
			return err

		case http.StatusNoContent:
			// success, empty body
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			return nil

		case http.StatusNotFound:
			resp.Body.Close()
			return ErrNotFound

		case http.StatusUnauthorized:
			resp.Body.Close()
			return ErrUnauthorized

		case http.StatusForbidden:
			resp.Body.Close()
			return ErrForbidden

		case http.StatusTooManyRequests, http.StatusInternalServerError,
			http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			// Prefer server-provided Retry-After; otherwise exponential backoff.
			wait := retryAfter(resp)
			resp.Body.Close()
			if wait == 0 {
				wait = backoff(i)
			}
			lastErr = fmt.Errorf("remote %d", resp.StatusCode)
			if i < 3 && sleepCtx(ctx, wait) {
				continue
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return lastErr

		default:
			// read a small error body for diagnostics
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			return fmt.Errorf("bad status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
		}
	}

	return lastErr
}

// sleepCtx waits for d or returns early if ctx is done.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// retryAfter parses Retry-After header (seconds or HTTP-date). Returns 0 if absent/invalid.
func retryAfter(resp *http.Response) time.Duration {
	h := resp.Header.Get("Retry-After")
	if h == "" {
		return 0
	}
	// seconds form
	if secs, err := strconv.Atoi(strings.TrimSpace(h)); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	// HTTP-date form
	if t, err := http.ParseTime(h); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

// backoff returns an exponential backoff delay with concurrency-safe jitter.
// i = retry attempt (0,1,2,...). Base doubles each attempt (200ms, 400ms, 800ms...),
// with up to +50% random jitter to avoid thundering herds.
func backoff(i int) time.Duration {
	base := time.Duration(1<<i) * 200 * time.Millisecond
	// concurrency-safe jitter using crypto/rand
	var b [1]byte
	if _, err := crand.Read(b[:]); err != nil {
		return base
	}
	f := float64(b[0]) / 255.0                  // 0..1
	j := time.Duration(0.5 * f * float64(base)) // up to +50%
	return base + j
}
