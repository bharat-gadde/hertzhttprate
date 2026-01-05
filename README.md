# hertzhttprate - HTTP Rate Limiter for Hertz

[![Go Reference](https://pkg.go.dev/badge/github.com/bharat-gadde/hertzhttprate.svg)](https://pkg.go.dev/github.com/bharat-gadde/hertzhttprate)
[![Go Report Card](https://goreportcard.com/badge/github.com/bharat-gadde/hertzhttprate)](https://goreportcard.com/report/github.com/bharat-gadde/hertzhttprate)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A high-performance HTTP rate limiter middleware for [CloudWeGo Hertz](https://github.com/cloudwego/hertz) web framework. This package is a port of [go-chi/httprate](https://github.com/go-chi/httprate) adapted for Hertz's middleware pattern.

## Features

- 🚀 **High Performance** - Built on the efficient Sliding Window Counter algorithm
- 🎯 **Flexible Key Functions** - Rate limit by IP, endpoint, custom headers, or any arbitrary key
- 📊 **Standard Headers** - Automatic `X-RateLimit-*` response headers
- 🔧 **Highly Configurable** - Custom limit handlers, error handlers, and response headers
- 🔌 **Pluggable Backend** - Uses [go-chi/httprate](https://github.com/go-chi/httprate)'s `LimitCounter` interface for custom backends (Redis, etc.)
- ✅ **Well Tested** - Comprehensive test coverage

## Algorithm

The rate limiter uses the **Sliding Window Counter** pattern, inspired by [CloudFlare's approach](https://blog.cloudflare.com/counting-things-a-lot-of-different-things). This algorithm:

- Provides accurate rate limiting
- Smooths traffic spikes
- Supports distributed rate limiting across multiple servers when using a shared backend like Redis

## Installation

```bash
go get github.com/bharat-gadde/hertzhttprate
```

## Quick Start

```go
package main

import (
    "context"
    "time"

    "github.com/bharat-gadde/hertzhttprate"
    "github.com/cloudwego/hertz/pkg/app"
    "github.com/cloudwego/hertz/pkg/app/server"
)

func main() {
    h := server.Default()

    // Rate limit all requests at 100 req/min by IP address
    limiter := hertzhttprate.NewRateLimiter(100, time.Minute, hertzhttprate.WithKeyByIP())
    h.Use(hertzhttprate.Middleware(limiter))

    h.GET("/", func(ctx context.Context, c *app.RequestContext) {
        c.String(200, "Hello, World!")
    })

    h.Spin()
}
```

## Usage Examples

### Rate Limit by IP Address

```go
limiter := hertzhttprate.NewRateLimiter(100, time.Minute, hertzhttprate.WithKeyByIP())
h.Use(hertzhttprate.Middleware(limiter))
```

### Rate Limit by IP and Endpoint

```go
limiter := hertzhttprate.NewRateLimiter(
    10, 
    10*time.Second,
    hertzhttprate.WithKeyFuncs(hertzhttprate.KeyByIP, hertzhttprate.KeyByEndpoint),
)
h.Use(hertzhttprate.Middleware(limiter))
```

### Rate Limit by Custom Key (e.g., API Token)

```go
limiter := hertzhttprate.NewRateLimiter(
    100,
    time.Minute,
    hertzhttprate.WithKeyFuncs(func(ctx context.Context, c *app.RequestContext) (string, error) {
        return string(c.GetHeader("X-API-Token")), nil
    }),
)
h.Use(hertzhttprate.Middleware(limiter))
```

### Rate Limit by Request Payload (e.g., Login Endpoint)

```go
// Create a dedicated rate limiter for login
loginRateLimiter := hertzhttprate.NewRateLimiter(5, time.Minute)

h.POST("/login", func(ctx context.Context, c *app.RequestContext) {
    var payload struct {
        Username string `json:"username"`
        Password string `json:"password"`
    }
    if err := c.BindJSON(&payload); err != nil {
        c.JSON(400, map[string]string{"error": "invalid payload"})
        return
    }

    // Rate limit by username - 5 attempts per minute
    if loginRateLimiter.RespondOnLimit(ctx, c, payload.Username) {
        return
    }

    // Process login...
    c.JSON(200, map[string]string{"status": "ok"})
})
```

### Custom Response for Rate-Limited Requests

The default response is `HTTP 429` with `Too Many Requests` body. Override it with:

```go
limiter := hertzhttprate.NewRateLimiter(
    10,
    time.Minute,
    hertzhttprate.WithLimitHandler(func(ctx context.Context, c *app.RequestContext) {
        c.JSON(429, map[string]interface{}{
            "error":   "rate_limited",
            "message": "Too many requests. Please slow down.",
        })
        c.Abort()
    }),
)
```

### Custom Error Handler

```go
limiter := hertzhttprate.NewRateLimiter(
    10,
    time.Minute,
    hertzhttprate.WithErrorHandler(func(ctx context.Context, c *app.RequestContext, err error) {
        c.JSON(500, map[string]interface{}{
            "error":   "rate_limiter_error",
            "message": err.Error(),
        })
        c.Abort()
    }),
)
```

### Custom Response Headers

```go
limiter := hertzhttprate.NewRateLimiter(
    1000,
    time.Minute,
    hertzhttprate.WithResponseHeaders(hertzhttprate.ResponseHeaders{
        Limit:      "X-RateLimit-Limit",
        Remaining:  "X-RateLimit-Remaining",
        Reset:      "X-RateLimit-Reset",
        RetryAfter: "Retry-After",
        Increment:  "", // omit this header
    }),
)
```

### Disable Response Headers

```go
limiter := hertzhttprate.NewRateLimiter(
    1000,
    time.Minute,
    hertzhttprate.WithResponseHeaders(hertzhttprate.ResponseHeaders{}),
)
```

### Route Group Rate Limiting

```go
h := server.Default()

// Global rate limit
globalLimiter := hertzhttprate.NewRateLimiter(1000, time.Minute, hertzhttprate.WithKeyByIP())
h.Use(hertzhttprate.Middleware(globalLimiter))

// Stricter limit for API routes
apiGroup := h.Group("/api")
apiLimiter := hertzhttprate.NewRateLimiter(100, time.Minute, hertzhttprate.WithKeyByIP())
apiGroup.Use(hertzhttprate.Middleware(apiLimiter))

// Even stricter for admin routes
adminGroup := h.Group("/admin")
adminLimiter := hertzhttprate.NewRateLimiter(10, time.Minute, hertzhttprate.WithKeyByIP())
adminGroup.Use(hertzhttprate.Middleware(adminLimiter))
```

### Dynamic Rate Limiting with Context

You can override the rate limit per-request using context:

```go
h.Use(func(ctx context.Context, c *app.RequestContext) {
    // Premium users get higher limits
    if isPremiumUser(c) {
        ctx = hertzhttprate.WithRequestLimit(ctx, 1000)
    }
    c.Next(ctx)
})
h.Use(hertzhttprate.Middleware(limiter))
```

### Custom Increment per Request

```go
h.Use(func(ctx context.Context, c *app.RequestContext) {
    // Heavy operations cost more
    if isHeavyOperation(c) {
        ctx = hertzhttprate.WithIncrement(ctx, 10)
    }
    c.Next(ctx)
})
h.Use(hertzhttprate.Middleware(limiter))
```

## API Reference

### Functions

| Function | Description |
| -------- | ----------- |
| `NewRateLimiter(limit int, window time.Duration, opts ...Option)` | Creates a new rate limiter |
| `Middleware(limiter *RateLimiter)` | Returns Hertz middleware handler |
| `KeyByIP(ctx, c)` | Key function that returns client IP |
| `KeyByEndpoint(ctx, c)` | Key function that returns request path |
| `KeyByFullPath(ctx, c)` | Key function that returns full matched route path |

### Options

| Option | Description |
| ------ | ----------- |
| `WithKeyFuncs(fns ...KeyFunc)` | Set custom key functions |
| `WithKeyByIP()` | Shorthand for `WithKeyFuncs(KeyByIP)` |
| `WithKeyByEndpoint()` | Shorthand for `WithKeyFuncs(KeyByEndpoint)` |
| `WithLimitHandler(h app.HandlerFunc)` | Custom handler when rate limited |
| `WithErrorHandler(h func(...))` | Custom handler for errors |
| `WithLimitCounter(c LimitCounter)` | Custom backend counter (e.g., Redis) |
| `WithResponseHeaders(headers ResponseHeaders)` | Custom response header names |

### RateLimiter Methods

| Method | Description |
| ------ | ----------- |
| `OnLimit(ctx, c, key) bool` | Check limit without sending response |
| `RespondOnLimit(ctx, c, key) bool` | Check limit and send response if limited |
| `Status(key) (bool, float64, error)` | Get current rate status for a key |
| `Counter() LimitCounter` | Get the underlying counter |

### Context Functions

| Function | Description |
| -------- | ----------- |
| `WithIncrement(ctx, value int)` | Set custom increment for this request |
| `WithRequestLimit(ctx, value int)` | Override rate limit for this request |

## Response Headers

By default, the following headers are set on every response:

| Header | Description |
| ------ | ----------- |
| `X-RateLimit-Limit` | Maximum requests allowed in the window |
| `X-RateLimit-Remaining` | Remaining requests in current window |
| `X-RateLimit-Reset` | Unix timestamp when the window resets |
| `Retry-After` | Seconds until retry (only when rate limited) |
| `X-RateLimit-Increment` | Request cost (only when > 1) |

## Custom Backend

The rate limiter uses an in-memory counter by default. For distributed systems, implement the `LimitCounter` interface:

```go
type LimitCounter interface {
    Config(requestLimit int, windowLength time.Duration)
    Increment(key string, currentWindow time.Time) error
    IncrementBy(key string, currentWindow time.Time, amount int) error
    Get(key string, currentWindow, previousWindow time.Time) (int, int, error)
}
```

You can use backends compatible with [go-chi/httprate](https://github.com/go-chi/httprate), such as:

- [httprate-redis](https://github.com/go-chi/httprate-redis) (may require adapter)

## Credits

This package is a port of [go-chi/httprate](https://github.com/go-chi/httprate) for the [CloudWeGo Hertz](https://github.com/cloudwego/hertz) web framework.

The original `httprate` was created by the [go-chi](https://github.com/go-chi) team and implements the Sliding Window Counter algorithm inspired by [CloudFlare's rate limiting approach](https://blog.cloudflare.com/counting-things-a-lot-of-different-things).

## Related Projects

- [CloudWeGo Hertz](https://github.com/cloudwego/hertz) - High-performance HTTP framework
- [go-chi/httprate](https://github.com/go-chi/httprate) - Original rate limiter for go-chi
- [go-chi/httprate-redis](https://github.com/go-chi/httprate-redis) - Redis backend for httprate

## License

MIT License - see [LICENSE](LICENSE) file for details.
