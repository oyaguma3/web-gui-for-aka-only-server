package middleware

import (
	"net/http"

	"aka-webgui/internal/config"

	"github.com/gin-gonic/gin"
)

const SessionCookieName = "aka_webgui_session"

func AuthRequired(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		cookie, err := c.Cookie(SessionCookieName)
		if err != nil || cookie != "authenticated" {
			// Check if it's an HTMX request
			if c.GetHeader("HX-Request") == "true" {
				// For HTMX, send a redirect via header to avoid partial rendering
				c.Header("HX-Redirect", "/login")
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		c.Next()
	}
}

func SetSession(c *gin.Context, cfg *config.Config) {
	c.SetCookie(
		SessionCookieName,
		"authenticated",
		cfg.SessionMinutes*60,
		"/",
		"",    // domain
		false, // secure (set to true if HTTPS)
		true,  // httpOnly
	)
}

func ClearSession(c *gin.Context) {
	c.SetCookie(
		SessionCookieName,
		"",
		-1,
		"/",
		"",
		false,
		true,
	)
}
