package stash

import (
	"time"
)

// List represents a user's collection of URLs
type List struct {
	ID          string
	UserID      string
	Name        string
	Description string
	IsDefault   bool
	Position    int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// URL represents a saved URL (shared across users via hash dedup)
type URL struct {
	ID          string
	URLHash     string
	URL         string
	Title       string
	Description string
	FaviconURL  string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ListURL represents a URL's membership in a list with user-specific notes
type ListURL struct {
	ID        string
	ListID    string
	URLID     string
	Notes     string
	Position  int
	CreatedAt time.Time
	URL       *URL // Embedded for convenience when fetching
}
