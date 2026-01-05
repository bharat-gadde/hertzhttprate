package hertzhttprate

import (
	"context"
	"net"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
)

type KeyFunc func(ctx context.Context, r *app.RequestContext) (string, error)
type Option func(rl *RateLimiter)

// Set custom response headers. If empty, the header is omitted.
type ResponseHeaders struct {
	Limit      string // Default: X-RateLimit-Limit
	Remaining  string // Default: X-RateLimit-Remaining
	Increment  string // Default: X-RateLimit-Increment
	Reset      string // Default: X-RateLimit-Reset
	RetryAfter string // Default: Retry-After
}

func Key(key string) func(ctx context.Context, r *app.RequestContext) (string, error) {
	return func(ctx context.Context, r *app.RequestContext) (string, error) {
		return key, nil
	}
}

func KeyByIP(_ context.Context, r *app.RequestContext) (string, error) {
	return canonicalizeIP(r.ClientIP()), nil
}

func KeyByEndpoint(_ context.Context, r *app.RequestContext) (string, error) {
	return string(r.Path()), nil
}

func KeyByFullPath(_ context.Context, r *app.RequestContext) (string, error) {
	return r.FullPath(), nil
}

func WithKeyFuncs(keyFuncs ...KeyFunc) Option {
	return func(rl *RateLimiter) {
		if len(keyFuncs) > 0 {
			rl.keyFn = composedKeyFunc(keyFuncs...)
		}
	}
}

func WithKeyByIP() Option {
	return WithKeyFuncs(KeyByIP)
}

func WithKeyByFullPath() Option {
	return WithKeyFuncs(KeyByFullPath)
}

func WithKeyByEndpoint() Option {
	return WithKeyFuncs(KeyByEndpoint)
}

func WithLimitHandler(h app.HandlerFunc) Option {
	return func(rl *RateLimiter) {
		rl.onRateLimited = h
	}
}

func WithErrorHandler(h func(ctx context.Context, c *app.RequestContext, err error)) Option {
	return func(rl *RateLimiter) {
		rl.onError = h
	}
}

func WithLimitCounter(c LimitCounter) Option {
	return func(rl *RateLimiter) {
		rl.limitCounter = c
	}
}

func WithResponseHeaders(headers ResponseHeaders) Option {
	return func(rl *RateLimiter) {
		rl.headers = headers
	}
}

func WithNoop() Option {
	return func(rl *RateLimiter) {}
}

func composedKeyFunc(keyFuncs ...KeyFunc) KeyFunc {
	return func(ctx context.Context, r *app.RequestContext) (string, error) {
		var key strings.Builder
		for i := 0; i < len(keyFuncs); i++ {
			k, err := keyFuncs[i](ctx, r)
			if err != nil {
				return "", err
			}
			key.WriteString(k)
			key.WriteRune(':')
		}
		return key.String(), nil
	}
}

// canonicalizeIP returns a form of ip suitable for comparison to other IPs.
// For IPv4 addresses, this is simply the whole string.
// For IPv6 addresses, this is the /64 prefix.
func canonicalizeIP(ip string) string {
	isIPv6 := false
	// This is how net.ParseIP decides if an address is IPv6
	// https://cs.opensource.google/go/go/+/refs/tags/go1.17.7:src/net/ip.go;l=704
	for i := 0; !isIPv6 && i < len(ip); i++ {
		switch ip[i] {
		case '.':
			// IPv4
			return ip
		case ':':
			// IPv6
			isIPv6 = true
		}
	}
	if !isIPv6 {
		// Not an IP address at all
		return ip
	}

	ipv6 := net.ParseIP(ip)
	if ipv6 == nil {
		return ip
	}

	return ipv6.Mask(net.CIDRMask(64, 128)).String()
}
