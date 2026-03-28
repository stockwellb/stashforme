package handlers

import (
	"github.com/labstack/echo/v4"

	"stashforme/internal/auth"
)

// GetUser retrieves the authenticated user from context, returns nil if not authenticated
func GetUser(c echo.Context) *auth.User {
	user, ok := c.Get("user").(*auth.User)
	if !ok {
		return nil
	}
	return user
}

// RequireUser retrieves the authenticated user, returns nil if not found
// Handlers should redirect to login if nil is returned
func RequireUser(c echo.Context) *auth.User {
	return GetUser(c)
}
