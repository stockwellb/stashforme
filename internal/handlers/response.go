package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// Link represents a HATEOAS link
type Link struct {
	Href string `json:"href"`
}

// Response represents a standard API response with HATEOAS links
type Response struct {
	Data   any             `json:"data,omitempty"`
	Error  string          `json:"error,omitempty"`
	Links  map[string]Link `json:"_links,omitempty"`
}

// jsonError returns a JSON error response
func jsonError(c echo.Context, status int, message string) error {
	return c.JSON(status, Response{Error: message})
}

// jsonSuccess returns a JSON success response with optional links
func jsonSuccess(c echo.Context, data any, links map[string]Link) error {
	return c.JSON(http.StatusOK, Response{Data: data, Links: links})
}

// jsonSuccessWithStatus returns a JSON success response with a custom status code
func jsonSuccessWithStatus(c echo.Context, status int, data any, links map[string]Link) error {
	return c.JSON(status, Response{Data: data, Links: links})
}

// jsonData returns a JSON response with data only (no links)
func jsonData(c echo.Context, data any) error {
	return c.JSON(http.StatusOK, Response{Data: data})
}

// jsonRedirect returns a JSON response with a redirect link
func jsonRedirect(c echo.Context, href string) error {
	return c.JSON(http.StatusOK, Response{
		Links: map[string]Link{
			"redirect": {Href: href},
		},
	})
}
