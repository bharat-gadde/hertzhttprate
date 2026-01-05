package hertzhttprate

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/go-chi/httprate"
)

type LimitCounter interface {
	Config(requestLimit int, windowLength time.Duration)
	Increment(key string, currentWindow time.Time) error
	IncrementBy(key string, currentWindow time.Time, amount int) error
	Get(key string, currentWindow, previousWindow time.Time) (int, int, error)
}

func NewRateLimiter(requestLimit int, windowLength time.Duration, options ...Option) *RateLimiter {
	rl := &RateLimiter{
		requestLimit: requestLimit,
		windowLength: windowLength,
		headers: ResponseHeaders{
			Limit:      "X-RateLimit-Limit",
			Remaining:  "X-RateLimit-Remaining",
			Increment:  "X-RateLimit-Increment",
			Reset:      "X-RateLimit-Reset",
			RetryAfter: "Retry-After",
		},
	}

	for _, opt := range options {
		opt(rl)
	}

	if rl.keyFn == nil {
		rl.keyFn = Key("*")
	}

	if rl.limitCounter == nil {
		rl.limitCounter = httprate.NewLocalLimitCounter(windowLength)
	} else {
		rl.limitCounter.Config(requestLimit, windowLength)
	}

	if rl.onRateLimited == nil {
		rl.onRateLimited = onRateLimited
	}

	if rl.onError == nil {
		rl.onError = onError
	}

	return rl
}

type RateLimiter struct {
	requestLimit  int
	windowLength  time.Duration
	keyFn         KeyFunc
	limitCounter  LimitCounter
	onRateLimited func(ctx context.Context, c *app.RequestContext)
	onError       func(ctx context.Context, c *app.RequestContext, err error)
	headers       ResponseHeaders
	mu            sync.Mutex
}

// OnLimit checks the rate limit for the given key and updates the response headers accordingly.
// If the limit is reached, it returns true, indicating that the request should be halted. Otherwise,
// it increments the request count and returns false. This method does not send an HTTP response,
// so the caller must handle the response themselves or use the RespondOnLimit() method instead.
func (l *RateLimiter) OnLimit(ctx context.Context, r *app.RequestContext, key string) bool {
	currentWindow := time.Now().UTC().Truncate(l.windowLength)

	limit := l.requestLimit
	if val := getRequestLimit(ctx); val > 0 {
		limit = val
	}
	r.Header(l.headers.Limit, strconv.Itoa(limit))
	r.Header(l.headers.Reset, strconv.FormatInt(currentWindow.Add(l.windowLength).Unix(), 10))

	l.mu.Lock()
	_, rateFloat, err := l.calculateRate(key, limit)
	if err != nil {
		l.mu.Unlock()
		l.onError(ctx, r, err)
		return true
	}
	rate := int(math.Round(rateFloat))

	increment := getIncrement(ctx)
	if increment > 1 {
		r.Header(l.headers.Increment, strconv.Itoa(increment))
	}

	if rate+increment > limit {
		r.Header(l.headers.Remaining, strconv.Itoa(limit-rate))

		l.mu.Unlock()
		r.Header(l.headers.RetryAfter, strconv.Itoa(int(l.windowLength.Seconds()))) // RFC 6585
		return true
	}

	err = l.limitCounter.IncrementBy(key, currentWindow, increment)
	if err != nil {
		l.mu.Unlock()
		l.onError(ctx, r, err)
		return true
	}
	l.mu.Unlock()

	r.Header(l.headers.Remaining, strconv.Itoa(limit-rate-increment))
	return false
}

// RespondOnLimit checks the rate limit for the given key and updates the response headers accordingly.
// If the limit is reached, it automatically sends an HTTP response and returns true, signaling the
// caller to halt further request processing. If the limit is not reached, it increments the request
// count and returns false, allowing the request to proceed.
func (l *RateLimiter) RespondOnLimit(ctx context.Context, r *app.RequestContext, key string) bool {
	onLimit := l.OnLimit(ctx, r, key)
	if onLimit {
		l.onRateLimited(ctx, r)
	}
	return onLimit
}

func (l *RateLimiter) Counter() LimitCounter {
	return l.limitCounter
}

func (l *RateLimiter) Status(key string) (bool, float64, error) {
	return l.calculateRate(key, l.requestLimit)
}

func Middleware(l *RateLimiter) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		key, err := l.keyFn(ctx, c)
		if err != nil {
			l.onError(ctx, c, err)
			return
		}

		if l.RespondOnLimit(ctx, c, key) {
			return
		}

		c.Next(ctx)
	}
}

func (l *RateLimiter) calculateRate(key string, requestLimit int) (bool, float64, error) {
	now := time.Now().UTC()
	currentWindow := now.Truncate(l.windowLength)
	previousWindow := currentWindow.Add(-l.windowLength)

	currCount, prevCount, err := l.limitCounter.Get(key, currentWindow, previousWindow)
	if err != nil {
		return false, 0, err
	}

	diff := now.Sub(currentWindow)
	rate := float64(prevCount)*(float64(l.windowLength)-float64(diff))/float64(l.windowLength) + float64(currCount)
	if rate > float64(requestLimit) {
		return false, rate, nil
	}

	return true, rate, nil
}

func onRateLimited(_ context.Context, r *app.RequestContext) {
	r.SetStatusCode(http.StatusTooManyRequests)
	r.SetBodyString(http.StatusText(http.StatusTooManyRequests))
	r.Abort()
}

func onError(_ context.Context, r *app.RequestContext, err error) {
	r.SetStatusCode(http.StatusPreconditionRequired)
	r.SetBodyString(err.Error())
	r.Abort()
}
