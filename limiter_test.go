package hertzhttprate

import (
	"bytes"
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/config"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/route"
)

func newTestEngine() *route.Engine {
	opt := config.NewOptions(nil)
	return route.NewEngine(opt)
}

func TestLimit(t *testing.T) {
	type test struct {
		name          string
		requestsLimit int
		windowLength  time.Duration
		respCodes     []int
	}
	tests := []test{
		{
			name:          "no-block",
			requestsLimit: 3,
			windowLength:  4 * time.Second,
			respCodes:     []int{200, 200, 200},
		},
		{
			name:          "block",
			requestsLimit: 3,
			windowLength:  2 * time.Second,
			respCodes:     []int{200, 200, 200, 429},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestEngine()
			limiter := NewRateLimiter(tt.requestsLimit, tt.windowLength)
			h.Use(Middleware(limiter))
			h.GET("/", func(ctx context.Context, c *app.RequestContext) {
				c.String(http.StatusOK, "OK")
			})

			for i, code := range tt.respCodes {
				w := ut.PerformRequest(h, "GET", "/", nil)
				if respCode := w.Code; respCode != code {
					t.Errorf("resp.StatusCode(%v) = %v, want %v", i, respCode, code)
				}
			}
		})
	}
}

func TestWithIncrement(t *testing.T) {
	type test struct {
		name          string
		increment     int
		requestsLimit int
		respCodes     []int
	}
	tests := []test{
		{
			name:          "no limit",
			increment:     0,
			requestsLimit: 3,
			respCodes:     []int{200, 200, 200, 200},
		},
		{
			name:          "increment 1",
			increment:     1,
			requestsLimit: 3,
			respCodes:     []int{200, 200, 200, 429},
		},
		{
			name:          "increment 2",
			increment:     2,
			requestsLimit: 3,
			respCodes:     []int{200, 429, 429, 429},
		},
		{
			name:          "increment 3",
			increment:     3,
			requestsLimit: 3,
			respCodes:     []int{200, 429, 429, 429},
		},
		{
			name:          "always block",
			increment:     4,
			requestsLimit: 3,
			respCodes:     []int{429, 429, 429, 429},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestEngine()
			limiter := NewRateLimiter(tt.requestsLimit, time.Minute)

			// Middleware to set increment on context
			h.Use(func(ctx context.Context, c *app.RequestContext) {
				ctx = WithIncrement(ctx, tt.increment)
				c.Next(ctx)
			})
			h.Use(Middleware(limiter))
			h.GET("/", func(ctx context.Context, c *app.RequestContext) {
				c.String(http.StatusOK, "OK")
			})

			for i, code := range tt.respCodes {
				w := ut.PerformRequest(h, "GET", "/", nil)
				if respCode := w.Code; respCode != code {
					t.Errorf("resp.StatusCode(%v) = %v, want %v", i, respCode, code)
				}
			}
		})
	}
}

func TestResponseHeaders(t *testing.T) {
	type test struct {
		name                string
		requestsLimit       int
		increments          []int
		respCodes           []int
		respLimitHeader     []string
		respRemainingHeader []string
	}
	tests := []test{
		{
			name:                "const increments",
			requestsLimit:       5,
			increments:          []int{1, 1, 1, 1, 1, 1},
			respCodes:           []int{200, 200, 200, 200, 200, 429},
			respLimitHeader:     []string{"5", "5", "5", "5", "5", "5"},
			respRemainingHeader: []string{"4", "3", "2", "1", "0", "0"},
		},
		{
			name:                "varying increments",
			requestsLimit:       5,
			increments:          []int{2, 2, 1, 2, 10, 1},
			respCodes:           []int{200, 200, 200, 429, 429, 429},
			respLimitHeader:     []string{"5", "5", "5", "5", "5", "5"},
			respRemainingHeader: []string{"3", "1", "0", "0", "0", "0"},
		},
		{
			name:                "no limit",
			requestsLimit:       5,
			increments:          []int{0, 0, 0, 0, 0, 0},
			respCodes:           []int{200, 200, 200, 200, 200, 200},
			respLimitHeader:     []string{"5", "5", "5", "5", "5", "5"},
			respRemainingHeader: []string{"5", "5", "5", "5", "5", "5"},
		},
		{
			name:                "always block",
			requestsLimit:       5,
			increments:          []int{10, 10, 10, 10, 10, 10},
			respCodes:           []int{429, 429, 429, 429, 429, 429},
			respLimitHeader:     []string{"5", "5", "5", "5", "5", "5"},
			respRemainingHeader: []string{"5", "5", "5", "5", "5", "5"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := len(tt.increments)
			if count != len(tt.respCodes) || count != len(tt.respLimitHeader) || count != len(tt.respRemainingHeader) {
				t.Fatalf("invalid test case: increments(%v), respCodes(%v), respLimitHeader(%v) and respRemainingHeaders(%v) must have same size", len(tt.increments), len(tt.respCodes), len(tt.respLimitHeader), len(tt.respRemainingHeader))
			}

			h := newTestEngine()
			limiter := NewRateLimiter(tt.requestsLimit, time.Minute)

			// Middleware to read increment from X-Test-Increment header
			h.Use(func(ctx context.Context, c *app.RequestContext) {
				if incHeader := c.Request.Header.Get("X-Test-Increment"); incHeader != "" {
					if inc, err := strconv.Atoi(incHeader); err == nil {
						ctx = WithIncrement(ctx, inc)
					}
				}
				c.Next(ctx)
			})
			h.Use(Middleware(limiter))
			h.GET("/", func(ctx context.Context, c *app.RequestContext) {
				c.String(http.StatusOK, "OK")
			})

			for i := 0; i < count; i++ {
				w := ut.PerformRequest(h, "GET", "/", nil,
					ut.Header{Key: "X-Test-Increment", Value: strconv.Itoa(tt.increments[i])})

				if respCode := w.Code; respCode != tt.respCodes[i] {
					t.Errorf("resp.StatusCode(%v) = %v, want %v", i, respCode, tt.respCodes[i])
				}

				if limit := string(w.Header().Peek("X-RateLimit-Limit")); limit != tt.respLimitHeader[i] {
					t.Errorf("X-RateLimit-Limit(%v) = %v, want %v", i, limit, tt.respLimitHeader[i])
				}
				if remaining := string(w.Header().Peek("X-RateLimit-Remaining")); remaining != tt.respRemainingHeader[i] {
					t.Errorf("X-RateLimit-Remaining(%v) = %v, want %v", i, remaining, tt.respRemainingHeader[i])
				}

				reset := string(w.Header().Peek("X-RateLimit-Reset"))
				if resetUnixTime, err := strconv.ParseInt(reset, 10, 64); err != nil || resetUnixTime <= time.Now().Unix() {
					t.Errorf("X-RateLimit-Reset(%v) = %v, want unix timestamp in the future", i, reset)
				}
			}
		})
	}
}

func TestCustomResponseHeaders(t *testing.T) {
	type test struct {
		name    string
		headers ResponseHeaders
	}
	tests := []test{
		{
			name: "no headers",
			headers: ResponseHeaders{
				Limit:      "",
				Remaining:  "",
				Reset:      "",
				RetryAfter: "",
				Increment:  "",
			},
		},
		{
			name: "custom headers",
			headers: ResponseHeaders{
				Limit:      "RateLimit-Limit",
				Remaining:  "RateLimit-Remaining",
				Reset:      "RateLimit-Reset",
				RetryAfter: "RateLimit-Retry",
				Increment:  "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestEngine()
			limiter := NewRateLimiter(
				1,
				time.Minute,
				WithLimitHandler(func(ctx context.Context, c *app.RequestContext) {
					c.String(429, "Wow Slow Down Kiddo")
				}),
				WithResponseHeaders(tt.headers),
			)

			// Middleware to set increment to force Retry-After and X-RateLimit-Increment headers
			h.Use(func(ctx context.Context, c *app.RequestContext) {
				ctx = WithIncrement(ctx, 2)
				c.Next(ctx)
			})
			h.Use(Middleware(limiter))
			h.GET("/", func(ctx context.Context, c *app.RequestContext) {
				c.String(http.StatusOK, "OK")
			})

			w := ut.PerformRequest(h, "GET", "/", nil)

			for _, header := range []string{
				"X-RateLimit-Limit",
				"X-RateLimit-Remaining",
				"X-RateLimit-Increment",
				"X-RateLimit-Reset",
				"Retry-After",
			} {
				if len(w.Header().PeekAll(header)) != 0 {
					t.Errorf("%q header not expected", header)
				}
			}

			for _, header := range []string{
				tt.headers.Limit,
				tt.headers.Remaining,
				tt.headers.Increment,
				tt.headers.Reset,
				tt.headers.RetryAfter,
			} {
				if header == "" {
					continue
				}
				if h := string(w.Header().Peek(header)); h == "" {
					t.Errorf("%q header expected", header)
				}
			}
		})
	}
}

func TestLimitHandler(t *testing.T) {
	type test struct {
		name          string
		requestsLimit int
		windowLength  time.Duration
		responses     []struct {
			Body       string
			StatusCode int
		}
	}
	tests := []test{
		{
			name:          "no-block",
			requestsLimit: 3,
			windowLength:  4 * time.Second,
			responses: []struct {
				Body       string
				StatusCode int
			}{
				{Body: "", StatusCode: 200},
				{Body: "", StatusCode: 200},
				{Body: "", StatusCode: 200},
			},
		},
		{
			name:          "block",
			requestsLimit: 3,
			windowLength:  2 * time.Second,
			responses: []struct {
				Body       string
				StatusCode int
			}{
				{Body: "", StatusCode: 200},
				{Body: "", StatusCode: 200},
				{Body: "", StatusCode: 200},
				{Body: "Wow Slow Down Kiddo", StatusCode: 429},
			},
		},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestEngine()
			limiter := NewRateLimiter(
				tt.requestsLimit,
				tt.windowLength,
				WithLimitHandler(func(ctx context.Context, c *app.RequestContext) {
					c.SetStatusCode(429)
					c.SetBodyString("Wow Slow Down Kiddo")
					c.Abort()
				}),
			)
			h.Use(Middleware(limiter))
			h.GET("/", func(ctx context.Context, c *app.RequestContext) {
				c.String(http.StatusOK, "")
			})

			for j, expected := range tt.responses {
				w := ut.PerformRequest(h, "GET", "/", nil)
				if respStatus := w.Code; respStatus != expected.StatusCode {
					t.Errorf("resp.StatusCode(test=%v, req=%v) = %v, want %v", i, j, respStatus, expected.StatusCode)
				}
				respBody := strings.TrimSuffix(w.Body.String(), "\n")

				if respBody != expected.Body {
					t.Errorf("resp.Body(test=%v, req=%v) = %v, want %v", i, j, respBody, expected.Body)
				}
			}
		})
	}
}

func TestLimitIP(t *testing.T) {
	type test struct {
		name          string
		requestsLimit int
		windowLength  time.Duration
		reqIp         []string
		respCodes     []int
	}
	tests := []test{
		{
			name:          "no-block",
			requestsLimit: 3,
			windowLength:  2 * time.Second,
			reqIp:         []string{"1.1.1.1", "2.2.2.2"},
			respCodes:     []int{200, 200},
		},
		{
			name:          "block-ip",
			requestsLimit: 1,
			windowLength:  2 * time.Second,
			reqIp:         []string{"1.1.1.1", "1.1.1.1", "2.2.2.2"},
			respCodes:     []int{200, 429, 200},
		},
		{
			name:          "block-ipv6",
			requestsLimit: 1,
			windowLength:  2 * time.Second,
			reqIp:         []string{"2001:DB8::21f:5bff:febf:ce22:1111", "2001:DB8::21f:5bff:febf:ce22:2222", "2002:DB8::21f:5bff:febf:ce22:1111"},
			respCodes:     []int{200, 429, 200},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestEngine()
			limiter := NewRateLimiter(tt.requestsLimit, tt.windowLength, WithKeyByIP())
			h.Use(Middleware(limiter))
			h.GET("/", func(ctx context.Context, c *app.RequestContext) {
				c.String(http.StatusOK, "OK")
			})

			for i, code := range tt.respCodes {
				w := ut.PerformRequest(h, "GET", "/", nil,
					ut.Header{Key: "X-Real-IP", Value: tt.reqIp[i]})
				if respCode := w.Code; respCode != code {
					t.Errorf("resp.StatusCode(%v) = %v, want %v", i, respCode, code)
				}
			}
		})
	}
}

func TestOverrideRequestLimit(t *testing.T) {
	h := newTestEngine()
	limiter := NewRateLimiter(
		3,
		time.Minute,
		WithLimitHandler(func(ctx context.Context, c *app.RequestContext) {
			c.SetStatusCode(429)
			c.SetBodyString("Wow Slow Down Kiddo")
			c.Abort()
		}),
	)

	// Middleware to read request limit from X-Test-RequestLimit header
	h.Use(func(ctx context.Context, c *app.RequestContext) {
		if limitHeader := c.Request.Header.Get("X-Test-RequestLimit"); limitHeader != "" {
			if limit, err := strconv.Atoi(limitHeader); err == nil {
				ctx = WithRequestLimit(ctx, limit)
			}
		}
		c.Next(ctx)
	})
	h.Use(Middleware(limiter))
	h.GET("/", func(ctx context.Context, c *app.RequestContext) {
		c.String(http.StatusOK, "")
	})

	responses := []struct {
		StatusCode   int
		Body         string
		RequestLimit int // Default: 3
	}{
		{StatusCode: 200, Body: ""},
		{StatusCode: 429, Body: "Wow Slow Down Kiddo", RequestLimit: 1},
		{StatusCode: 200, Body: ""},
		{StatusCode: 200, Body: ""},
		{StatusCode: 429, Body: "Wow Slow Down Kiddo"},

		{StatusCode: 200, Body: "", RequestLimit: 5},
		{StatusCode: 200, Body: "", RequestLimit: 5},
		{StatusCode: 429, Body: "Wow Slow Down Kiddo", RequestLimit: 5},
	}
	for i, response := range responses {
		var headers []ut.Header
		if response.RequestLimit > 0 {
			headers = append(headers, ut.Header{Key: "X-Test-RequestLimit", Value: strconv.Itoa(response.RequestLimit)})
		}

		w := ut.PerformRequest(h, "GET", "/", nil, headers...)
		if respStatus := w.Code; respStatus != response.StatusCode {
			t.Errorf("resp.StatusCode(%v) = %v, want %v", i, respStatus, response.StatusCode)
		}
		respBody := strings.TrimSuffix(w.Body.String(), "\n")

		if respBody != response.Body {
			t.Errorf("resp.Body(%v) = %q, want %q", i, respBody, response.Body)
		}
	}
}

func TestRateLimitPayload(t *testing.T) {
	loginRateLimiter := NewRateLimiter(5, time.Minute)

	h := newTestEngine()
	h.POST("/login", func(ctx context.Context, c *app.RequestContext) {
		var payload struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := c.BindJSON(&payload); err != nil || payload.Username == "" || payload.Password == "" {
			c.String(400, "")
			return
		}

		// Rate-limit login at 5 req/min.
		if loginRateLimiter.RespondOnLimit(ctx, c, payload.Username) {
			return
		}

		c.String(http.StatusOK, "login at 5 req/min")
	})

	responses := []struct {
		StatusCode int
		Body       string
	}{
		{StatusCode: 200, Body: "login at 5 req/min"},
		{StatusCode: 200, Body: "login at 5 req/min"},
		{StatusCode: 200, Body: "login at 5 req/min"},
		{StatusCode: 200, Body: "login at 5 req/min"},
		{StatusCode: 200, Body: "login at 5 req/min"},
		{StatusCode: 429, Body: "Too Many Requests"},
		{StatusCode: 429, Body: "Too Many Requests"},
		{StatusCode: 429, Body: "Too Many Requests"},
	}
	for i, response := range responses {
		body := bytes.NewBufferString(`{"username":"alice","password":"***"}`)
		w := ut.PerformRequest(h, "POST", "/login", &ut.Body{Body: body, Len: body.Len()},
			ut.Header{Key: "Content-Type", Value: "application/json"})

		if respStatus := w.Code; respStatus != response.StatusCode {
			t.Errorf("resp.StatusCode(%v) = %v, want %v", i, respStatus, response.StatusCode)
		}
		respBody := strings.TrimSuffix(w.Body.String(), "\n")

		if respBody != response.Body {
			t.Errorf("resp.Body(%v) = %q, want %q", i, respBody, response.Body)
		}
	}
}
