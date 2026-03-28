package handlers

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

	"stashforme/internal/stash"
	"stashforme/internal/views"
)

// StashHandler handles stash-related endpoints
type StashHandler struct {
	lists    *stash.ListStore
	urls     *stash.URLStore
	listURLs *stash.ListURLStore
}

// NewStashHandler creates a new stash handler
func NewStashHandler(lists *stash.ListStore, urls *stash.URLStore, listURLs *stash.ListURLStore) *StashHandler {
	return &StashHandler{
		lists:    lists,
		urls:     urls,
		listURLs: listURLs,
	}
}

// Stash renders the main stash page
func (h *StashHandler) Stash(c echo.Context) error {
	user := RequireUser(c)
	if user == nil {
		return c.Redirect(http.StatusSeeOther, PathLogin)
	}

	ctx := c.Request().Context()

	// Ensure default list exists
	defaultList, err := h.lists.FindOrCreateDefault(ctx, user.ID)
	if err != nil {
		c.Logger().Error("Failed to find/create default list:", err)
		return c.String(http.StatusInternalServerError, "Something went wrong")
	}

	// Get all lists
	lists, err := h.lists.FindByUserID(ctx, user.ID)
	if err != nil {
		c.Logger().Error("Failed to find lists:", err)
		return c.String(http.StatusInternalServerError, "Something went wrong")
	}

	// Get selected list (from query param or default)
	selectedID := c.QueryParam("list")
	var selectedList *stash.List
	if selectedID != "" {
		selectedList, err = h.lists.FindByID(ctx, selectedID, user.ID)
		if err != nil {
			selectedList = defaultList
		}
	} else {
		selectedList = defaultList
	}

	// Get URLs for selected list
	listURLs, err := h.listURLs.FindByListID(ctx, selectedList.ID)
	if err != nil {
		c.Logger().Error("Failed to find list urls:", err)
		listURLs = []*stash.ListURL{}
	}

	return Render(c, http.StatusOK, views.Stash(user, lists, selectedList, listURLs))
}

// ListDetail returns the content for a selected list (HTMX partial)
func (h *StashHandler) ListDetail(c echo.Context) error {
	user := RequireUser(c)
	if user == nil {
		return c.String(http.StatusUnauthorized, "Unauthorized")
	}

	ctx := c.Request().Context()
	listID := c.Param("id")

	list, err := h.lists.FindByID(ctx, listID, user.ID)
	if err != nil {
		return c.String(http.StatusNotFound, "List not found")
	}

	listURLs, err := h.listURLs.FindByListID(ctx, list.ID)
	if err != nil {
		c.Logger().Error("Failed to find list urls:", err)
		listURLs = []*stash.ListURL{}
	}

	return Render(c, http.StatusOK, views.ListContent(list, listURLs))
}

// NewListPage renders the page to create a new list
func (h *StashHandler) NewListPage(c echo.Context) error {
	user := RequireUser(c)
	if user == nil {
		return c.Redirect(http.StatusSeeOther, PathLogin)
	}
	return Render(c, http.StatusOK, views.NewListPage(user))
}

// CreateList creates a new list and redirects to it
func (h *StashHandler) CreateList(c echo.Context) error {
	user := RequireUser(c)
	if user == nil {
		return c.Redirect(http.StatusSeeOther, PathLogin)
	}

	name := c.FormValue("name")
	if name == "" {
		name = "New List"
	}
	description := c.FormValue("description")

	list, err := h.lists.Create(c.Request().Context(), user.ID, name, description)
	if err != nil {
		c.Logger().Error("Failed to create list:", err)
		return c.String(http.StatusInternalServerError, "Failed to create list")
	}

	// Redirect to the new list
	return c.Redirect(http.StatusSeeOther, fmt.Sprintf("/my/stash?list=%s", list.ID))
}

// UpdateList updates a list's name and description
func (h *StashHandler) UpdateList(c echo.Context) error {
	user := RequireUser(c)
	if user == nil {
		return c.String(http.StatusUnauthorized, "Unauthorized")
	}

	listID := c.Param("id")
	name := c.FormValue("name")
	description := c.FormValue("description")

	if name == "" {
		return c.String(http.StatusBadRequest, "Name is required")
	}

	list, err := h.lists.Update(c.Request().Context(), listID, user.ID, name, description)
	if err != nil {
		c.Logger().Error("Failed to update list:", err)
		return c.String(http.StatusInternalServerError, "Failed to update list")
	}

	// Return updated nav item
	return Render(c, http.StatusOK, views.ListNavItem(list, true))
}

// DeleteList soft-deletes a list
func (h *StashHandler) DeleteList(c echo.Context) error {
	user := RequireUser(c)
	if user == nil {
		return c.String(http.StatusUnauthorized, "Unauthorized")
	}

	listID := c.Param("id")

	err := h.lists.Delete(c.Request().Context(), listID, user.ID)
	if err != nil {
		if err == stash.ErrCannotDeleteDefaultList {
			return c.String(http.StatusBadRequest, "Cannot delete default list")
		}
		c.Logger().Error("Failed to delete list:", err)
		return c.String(http.StatusInternalServerError, "Failed to delete list")
	}

	// Redirect to stash page via HX-Redirect header
	c.Response().Header().Set("HX-Redirect", PathMyStash)
	return c.NoContent(http.StatusOK)
}

// NewURLPage renders the page to add a URL to a list
func (h *StashHandler) NewURLPage(c echo.Context) error {
	user := RequireUser(c)
	if user == nil {
		return c.Redirect(http.StatusSeeOther, PathLogin)
	}

	ctx := c.Request().Context()
	listID := c.Param("id")

	list, err := h.lists.FindByID(ctx, listID, user.ID)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, PathMyStash)
	}

	errorMsg := c.QueryParam("error")
	return Render(c, http.StatusOK, views.NewURLPage(user, list, errorMsg))
}

// AddURL adds a URL to a list and redirects back
func (h *StashHandler) AddURL(c echo.Context) error {
	user := RequireUser(c)
	if user == nil {
		return c.Redirect(http.StatusSeeOther, PathLogin)
	}

	ctx := c.Request().Context()
	listID := c.Param("id")

	// Verify user owns the list
	list, err := h.lists.FindByID(ctx, listID, user.ID)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, PathMyStash)
	}

	rawURL := c.FormValue("url")
	if rawURL == "" {
		return c.Redirect(http.StatusSeeOther, fmt.Sprintf("/my/stash/%s/new?error=URL+is+required", list.ID))
	}
	notes := c.FormValue("notes")

	// Find or create the URL
	url, _, err := h.urls.FindOrCreate(ctx, rawURL)
	if err != nil {
		c.Logger().Error("Failed to find/create url:", err)
		return c.Redirect(http.StatusSeeOther, fmt.Sprintf("/my/stash/%s/new?error=Failed+to+save+URL", list.ID))
	}

	// Add to list
	_, err = h.listURLs.Add(ctx, list.ID, url.ID, notes)
	if err != nil {
		if err == stash.ErrURLAlreadyInList {
			return c.Redirect(http.StatusSeeOther, fmt.Sprintf("/my/stash/%s/new?error=URL+already+in+list", list.ID))
		}
		c.Logger().Error("Failed to add url to list:", err)
		return c.Redirect(http.StatusSeeOther, fmt.Sprintf("/my/stash/%s/new?error=Failed+to+add+URL", list.ID))
	}

	// Redirect back to the list
	return c.Redirect(http.StatusSeeOther, fmt.Sprintf("/my/stash?list=%s", list.ID))
}

// UpdateListURL updates the URL and notes for a list URL entry
func (h *StashHandler) UpdateURLNotes(c echo.Context) error {
	user := RequireUser(c)
	if user == nil {
		return c.String(http.StatusUnauthorized, "Unauthorized")
	}

	ctx := c.Request().Context()
	listURLID := c.Param("id")
	rawURL := c.FormValue("url")
	notes := c.FormValue("notes")

	// Find the list URL and verify ownership
	listURL, err := h.listURLs.FindByIDWithURL(ctx, listURLID)
	if err != nil {
		return c.String(http.StatusNotFound, "URL not found")
	}

	// Verify user owns the list
	_, err = h.lists.FindByID(ctx, listURL.ListID, user.ID)
	if err != nil {
		return c.String(http.StatusNotFound, "URL not found")
	}

	// Find or create the URL (handles normalization and hashing)
	url, _, err := h.urls.FindOrCreate(ctx, rawURL)
	if err != nil {
		c.Logger().Error("Failed to find/create url:", err)
		return Render(c, http.StatusOK, views.EditNotesForm(listURL, "Invalid URL"))
	}

	// Update the list URL entry
	listURL, err = h.listURLs.Update(ctx, listURLID, url.ID, notes)
	if err != nil {
		if err == stash.ErrURLAlreadyInList {
			listURL, _ = h.listURLs.FindByIDWithURL(ctx, listURLID)
			return Render(c, http.StatusOK, views.EditNotesForm(listURL, "URL already in this list"))
		}
		c.Logger().Error("Failed to update list url:", err)
		return c.String(http.StatusInternalServerError, "Failed to update")
	}

	// Refetch with URL data
	listURL, _ = h.listURLs.FindByIDWithURL(ctx, listURLID)

	// Return updated item
	return Render(c, http.StatusOK, views.URLItem(listURL))
}

// EditURLNotesForm returns the edit form for URL notes (HTMX)
func (h *StashHandler) EditURLNotesForm(c echo.Context) error {
	user := RequireUser(c)
	if user == nil {
		return c.String(http.StatusUnauthorized, "Unauthorized")
	}

	ctx := c.Request().Context()
	listURLID := c.Param("id")

	// Find the list URL
	listURL, err := h.listURLs.FindByIDWithURL(ctx, listURLID)
	if err != nil {
		return c.String(http.StatusNotFound, "URL not found")
	}

	// Verify user owns the list
	_, err = h.lists.FindByID(ctx, listURL.ListID, user.ID)
	if err != nil {
		return c.String(http.StatusNotFound, "URL not found")
	}

	return Render(c, http.StatusOK, views.EditNotesForm(listURL, ""))
}

// URLItem returns a single URL item (for HTMX refresh after cancel)
func (h *StashHandler) URLItem(c echo.Context) error {
	user := RequireUser(c)
	if user == nil {
		return c.String(http.StatusUnauthorized, "Unauthorized")
	}

	ctx := c.Request().Context()
	listURLID := c.Param("id")

	// Find the list URL
	listURL, err := h.listURLs.FindByIDWithURL(ctx, listURLID)
	if err != nil {
		return c.String(http.StatusNotFound, "URL not found")
	}

	// Verify user owns the list
	_, err = h.lists.FindByID(ctx, listURL.ListID, user.ID)
	if err != nil {
		return c.String(http.StatusNotFound, "URL not found")
	}

	return Render(c, http.StatusOK, views.URLItem(listURL))
}

// RemoveURL removes a URL from a list
func (h *StashHandler) RemoveURL(c echo.Context) error {
	user := RequireUser(c)
	if user == nil {
		return c.String(http.StatusUnauthorized, "Unauthorized")
	}

	ctx := c.Request().Context()
	listURLID := c.Param("id")

	// Find the list URL
	listURL, err := h.listURLs.FindByID(ctx, listURLID)
	if err != nil {
		return c.String(http.StatusNotFound, "URL not found")
	}

	// Verify user owns the list
	_, err = h.lists.FindByID(ctx, listURL.ListID, user.ID)
	if err != nil {
		return c.String(http.StatusNotFound, "URL not found")
	}

	// Remove
	if err := h.listURLs.Remove(ctx, listURLID); err != nil {
		c.Logger().Error("Failed to remove url:", err)
		return c.String(http.StatusInternalServerError, "Failed to remove URL")
	}

	// Return empty content for HTMX to swap out
	return c.NoContent(http.StatusOK)
}
