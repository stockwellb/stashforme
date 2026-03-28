package stash

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
)

// URLStore handles URL persistence with hash-based deduplication
type URLStore struct {
	db *sql.DB
}

// NewURLStore creates a new URL store
func NewURLStore(db *sql.DB) *URLStore {
	return &URLStore{db: db}
}

// FindByID finds a URL by ID
func (s *URLStore) FindByID(ctx context.Context, id string) (*URL, error) {
	var u URL
	var title, description, faviconURL sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, url_hash, url, title, description, favicon_url, created_at, updated_at
		FROM urls
		WHERE id = $1
	`, id).Scan(
		&u.ID, &u.URLHash, &u.URL, &title, &description, &faviconURL,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrURLNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find url: %w", err)
	}
	u.Title = title.String
	u.Description = description.String
	u.FaviconURL = faviconURL.String
	return &u, nil
}

// FindByHash finds a URL by its hash
func (s *URLStore) FindByHash(ctx context.Context, urlHash string) (*URL, error) {
	var u URL
	var title, description, faviconURL sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, url_hash, url, title, description, favicon_url, created_at, updated_at
		FROM urls
		WHERE url_hash = $1
	`, urlHash).Scan(
		&u.ID, &u.URLHash, &u.URL, &title, &description, &faviconURL,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrURLNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find url by hash: %w", err)
	}
	u.Title = title.String
	u.Description = description.String
	u.FaviconURL = faviconURL.String
	return &u, nil
}

// FindOrCreate finds an existing URL by hash or creates a new one
func (s *URLStore) FindOrCreate(ctx context.Context, rawURL string) (*URL, bool, error) {
	normalized := NormalizeURL(rawURL)
	hash := HashURL(normalized)

	// Try to find existing
	u, err := s.FindByHash(ctx, hash)
	if err == nil {
		return u, false, nil
	}
	if err != ErrURLNotFound {
		return nil, false, err
	}

	// Create new
	var newURL URL
	var title, description, faviconURL sql.NullString
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO urls (url_hash, url)
		VALUES ($1, $2)
		ON CONFLICT (url_hash) DO UPDATE SET url_hash = urls.url_hash
		RETURNING id, url_hash, url, title, description, favicon_url, created_at, updated_at
	`, hash, normalized).Scan(
		&newURL.ID, &newURL.URLHash, &newURL.URL, &title, &description, &faviconURL,
		&newURL.CreatedAt, &newURL.UpdatedAt,
	)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create url: %w", err)
	}
	newURL.Title = title.String
	newURL.Description = description.String
	newURL.FaviconURL = faviconURL.String

	return &newURL, true, nil
}

// UpdateMetadata updates a URL's title, description, and favicon
func (s *URLStore) UpdateMetadata(ctx context.Context, id, title, description, faviconURL string) error {
	var titleVal, descVal, faviconVal sql.NullString
	if title != "" {
		titleVal = sql.NullString{String: title, Valid: true}
	}
	if description != "" {
		descVal = sql.NullString{String: description, Valid: true}
	}
	if faviconURL != "" {
		faviconVal = sql.NullString{String: faviconURL, Valid: true}
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE urls
		SET title = $1, description = $2, favicon_url = $3, updated_at = NOW()
		WHERE id = $4
	`, titleVal, descVal, faviconVal, id)
	if err != nil {
		return fmt.Errorf("failed to update url metadata: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check update result: %w", err)
	}
	if rows == 0 {
		return ErrURLNotFound
	}
	return nil
}

// NormalizeURL normalizes a URL for consistent hashing
func NormalizeURL(rawURL string) string {
	// Add scheme if missing
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	// Lowercase host
	parsed.Host = strings.ToLower(parsed.Host)

	// Remove trailing slash from path (unless it's just "/")
	if parsed.Path != "/" {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	}

	// Remove default ports
	if parsed.Port() == "80" && parsed.Scheme == "http" {
		parsed.Host = parsed.Hostname()
	}
	if parsed.Port() == "443" && parsed.Scheme == "https" {
		parsed.Host = parsed.Hostname()
	}

	// Remove fragment
	parsed.Fragment = ""

	return parsed.String()
}

// HashURL creates a SHA256 hash of a URL
func HashURL(normalizedURL string) string {
	hash := sha256.Sum256([]byte(normalizedURL))
	return hex.EncodeToString(hash[:])
}
