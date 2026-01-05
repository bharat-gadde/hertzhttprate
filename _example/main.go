package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/bharat-gadde/hertzhttprate"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/utils"
)

func main() {
	h := server.Default(server.WithHostPorts(":3333"))

	// Rate-limit all routes at 1000 req/min by IP address.
	globalLimiter := hertzhttprate.NewRateLimiter(1000, time.Minute, hertzhttprate.WithKeyByIP())
	h.Use(hertzhttprate.Middleware(globalLimiter))

	// Admin routes with user-based rate limiting
	adminGroup := h.Group("/admin")
	{
		// Mock middleware to set userID on request context
		adminGroup.Use(func(ctx context.Context, c *app.RequestContext) {
			// Note: Using c.Set() to store values in Hertz's RequestContext
			c.Set("userID", "123")
			c.Next(ctx)
		})

		// Rate-limit admin routes at 10 req/s by userID.
		adminLimiter := hertzhttprate.NewRateLimiter(
			10, time.Second,
			hertzhttprate.WithKeyFuncs(func(ctx context.Context, r *app.RequestContext) (string, error) {
				token := r.Value("userID").(string)
				return token, nil
			}),
		)
		adminGroup.Use(hertzhttprate.Middleware(adminLimiter))

		adminGroup.GET("/", func(ctx context.Context, c *app.RequestContext) {
			c.String(http.StatusOK, "admin at 10 req/s\n")
		})
	}

	// Rate-limiter for login endpoint.
	loginRateLimiter := hertzhttprate.NewRateLimiter(5, time.Minute)

	h.POST("/login", func(ctx context.Context, c *app.RequestContext) {
		var payload struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := c.BindJSON(&payload); err != nil || payload.Username == "" || payload.Password == "" {
			c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
			return
		}

		// Rate-limit login at 5 req/min by username.
		if loginRateLimiter.RespondOnLimit(ctx, c, payload.Username) {
			return
		}

		c.String(http.StatusOK, "login at 5 req/min\n")
	})

	log.Printf("Serving at localhost:3333")
	log.Println()
	log.Printf("Try running:")
	log.Printf(`curl -v "http://localhost:3333?[0-1000]"`)
	log.Printf(`curl -v "http://localhost:3333/admin?[1-12]"`)
	log.Printf(`curl -v "http://localhost:3333/login?[1-8]" --data '{"username":"alice","password":"***"}'`)

	h.Spin()
}
