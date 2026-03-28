package handlers

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

	"stashforme/internal/views"
)

// Handler holds dependencies for HTTP handlers
type Handler struct{}

// New creates a new Handler instance
func New() *Handler {
	return &Handler{}
}

// Home redirects to /my/stash if logged in, otherwise shows landing page
func (h *Handler) Home(c echo.Context) error {
	if user := GetUser(c); user != nil {
		return c.Redirect(http.StatusSeeOther, PathMyStash)
	}
	return Render(c, http.StatusOK, views.Home())
}

// Ping is a simple health check endpoint
func (h *Handler) Ping(c echo.Context) error {
	return c.String(http.StatusOK, "pong!")
}

// Me renders the user's profile page
func (h *Handler) Me(c echo.Context) error {
	user := RequireUser(c)
	if user == nil {
		return c.Redirect(http.StatusSeeOther, PathLogin)
	}
	return Render(c, http.StatusOK, views.Me(user))
}

// Render is a helper function to render templ components
func Render(c echo.Context, status int, t templ.Component) error {
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	c.Response().WriteHeader(status)
	return t.Render(c.Request().Context(), c.Response().Writer)
}
