package middleware

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"stashforme/internal/auth"
)

// ContextKey is a type for context keys
type ContextKey string

const (
	// UserIDKey is the context key for the authenticated user ID
	UserIDKey ContextKey = "user_id"
	// UserKey is the context key for the authenticated user
	UserKey ContextKey = "user"
)

// AuthConfig holds configuration for the auth middleware
type AuthConfig struct {
	Skipper  func(c echo.Context) bool
	Sessions *auth.SessionStore
	Users    *auth.UserStore
}

// DefaultSkipper returns true for public routes
func DefaultSkipper(c echo.Context) bool {
	path := c.Request().URL.Path
	// Public routes
	publicPaths := []string{
		"/",
		"/login",
		"/verify",
		"/passkey/setup",
		"/auth/send-code",
		"/auth/verify-code",
		"/auth/skip-passkey",
		"/auth/passkey/register",
		"/auth/passkey/login",
		"/auth/passkey/login/finish",
		"/api/ping",
		"/static",
	}
	for _, p := range publicPaths {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

// Auth returns middleware that validates session-based authentication
func Auth(sessions *auth.SessionStore, users *auth.UserStore) echo.MiddlewareFunc {
	return AuthWithConfig(AuthConfig{
		Skipper:  DefaultSkipper,
		Sessions: sessions,
		Users:    users,
	})
}

// AuthWithConfig returns auth middleware with custom config
func AuthWithConfig(config AuthConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if config.Skipper != nil && config.Skipper(c) {
				return next(c)
			}

			// Get session cookie
			cookie, err := c.Cookie(auth.SessionCookieName)
			if err != nil {
				return redirectToLogin(c)
			}

			// Validate session
			userID, err := config.Sessions.Validate(c.Request().Context(), cookie.Value)
			if err != nil {
				// Clear invalid session cookie
				c.SetCookie(&http.Cookie{
					Name:     auth.SessionCookieName,
					Value:    "",
					Path:     "/",
					HttpOnly: true,
					MaxAge:   -1,
				})
				return redirectToLogin(c)
			}

			// Store user ID in context
			c.Set(string(UserIDKey), userID)

			// Optionally load full user
			if config.Users != nil {
				user, err := config.Users.FindByID(c.Request().Context(), userID)
				if err == nil {
					c.Set(string(UserKey), user)
				}
			}

			return next(c)
		}
	}
}

// redirectToLogin redirects to login page or returns 401 for API requests
func redirectToLogin(c echo.Context) error {
	// For HTMX requests, send redirect header
	if c.Request().Header.Get("HX-Request") == "true" {
		c.Response().Header().Set("HX-Redirect", "/login")
		return c.NoContent(http.StatusUnauthorized)
	}

	// For API requests, return 401
	if strings.HasPrefix(c.Path(), "/api/") {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "authentication required",
		})
	}

	// For regular requests, redirect
	return c.Redirect(http.StatusSeeOther, "/login")
}

// GetUserID extracts the user ID from context
func GetUserID(c echo.Context) string {
	if id, ok := c.Get(string(UserIDKey)).(string); ok {
		return id
	}
	return ""
}

// GetUser extracts the user from context
func GetUser(c echo.Context) *auth.User {
	if user, ok := c.Get(string(UserKey)).(*auth.User); ok {
		return user
	}
	return nil
}
