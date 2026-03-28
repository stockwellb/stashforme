package stash

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// ListURLStore handles URL membership in lists
type ListURLStore struct {
	db *sql.DB
}

// NewListURLStore creates a new list URL store
func NewListURLStore(db *sql.DB) *ListURLStore {
	return &ListURLStore{db: db}
}

// FindByID finds a list URL by ID
func (s *ListURLStore) FindByID(ctx context.Context, id string) (*ListURL, error) {
	var lu ListURL
	var notes sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, list_id, url_id, notes, position, created_at
		FROM list_urls
		WHERE id = $1 AND deleted_at IS NULL
	`, id).Scan(
		&lu.ID, &lu.ListID, &lu.URLID, &notes, &lu.Position, &lu.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrListURLNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find list url: %w", err)
	}
	lu.Notes = notes.String
	return &lu, nil
}

// FindByIDWithURL finds a list URL by ID and includes the URL data
func (s *ListURLStore) FindByIDWithURL(ctx context.Context, id string) (*ListURL, error) {
	var lu ListURL
	var u URL
	var notes, title, description, faviconURL sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT
			lu.id, lu.list_id, lu.url_id, lu.notes, lu.position, lu.created_at,
			u.id, u.url_hash, u.url, u.title, u.description, u.favicon_url, u.created_at, u.updated_at
		FROM list_urls lu
		JOIN urls u ON u.id = lu.url_id
		WHERE lu.id = $1 AND lu.deleted_at IS NULL
	`, id).Scan(
		&lu.ID, &lu.ListID, &lu.URLID, &notes, &lu.Position, &lu.CreatedAt,
		&u.ID, &u.URLHash, &u.URL, &title, &description, &faviconURL, &u.CreatedAt, &u.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrListURLNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find list url with url: %w", err)
	}
	lu.Notes = notes.String
	u.Title = title.String
	u.Description = description.String
	u.FaviconURL = faviconURL.String
	lu.URL = &u
	return &lu, nil
}

// FindByListID finds all URLs in a list
func (s *ListURLStore) FindByListID(ctx context.Context, listID string) ([]*ListURL, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			lu.id, lu.list_id, lu.url_id, lu.notes, lu.position, lu.created_at,
			u.id, u.url_hash, u.url, u.title, u.description, u.favicon_url, u.created_at, u.updated_at
		FROM list_urls lu
		JOIN urls u ON u.id = lu.url_id
		WHERE lu.list_id = $1 AND lu.deleted_at IS NULL
		ORDER BY lu.position ASC, lu.created_at DESC
	`, listID)
	if err != nil {
		return nil, fmt.Errorf("failed to find list urls: %w", err)
	}
	defer rows.Close()

	var listURLs []*ListURL
	for rows.Next() {
		var lu ListURL
		var u URL
		var notes, title, description, faviconURL sql.NullString

		if err := rows.Scan(
			&lu.ID, &lu.ListID, &lu.URLID, &notes, &lu.Position, &lu.CreatedAt,
			&u.ID, &u.URLHash, &u.URL, &title, &description, &faviconURL, &u.CreatedAt, &u.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan list url: %w", err)
		}
		lu.Notes = notes.String
		u.Title = title.String
		u.Description = description.String
		u.FaviconURL = faviconURL.String
		lu.URL = &u
		listURLs = append(listURLs, &lu)
	}
	return listURLs, rows.Err()
}

// Add adds a URL to a list
func (s *ListURLStore) Add(ctx context.Context, listID, urlID, notes string) (*ListURL, error) {
	var lu ListURL
	var notesVal sql.NullString
	if notes != "" {
		notesVal = sql.NullString{String: notes, Valid: true}
	}

	err := s.db.QueryRowContext(ctx, `
		INSERT INTO list_urls (list_id, url_id, notes, position)
		VALUES ($1, $2, $3, (SELECT COALESCE(MAX(position), 0) + 1 FROM list_urls WHERE list_id = $1 AND deleted_at IS NULL))
		RETURNING id, list_id, url_id, notes, position, created_at
	`, listID, urlID, notesVal).Scan(
		&lu.ID, &lu.ListID, &lu.URLID, &notesVal, &lu.Position, &lu.CreatedAt,
	)
	if err != nil {
		// Check for unique constraint violation
		if isUniqueViolation(err) {
			return nil, ErrURLAlreadyInList
		}
		return nil, fmt.Errorf("failed to add url to list: %w", err)
	}
	lu.Notes = notesVal.String
	return &lu, nil
}

// Update updates the URL reference and notes for a list URL entry
func (s *ListURLStore) Update(ctx context.Context, id, urlID, notes string) (*ListURL, error) {
	var lu ListURL
	var notesVal sql.NullString
	if notes != "" {
		notesVal = sql.NullString{String: notes, Valid: true}
	}

	err := s.db.QueryRowContext(ctx, `
		UPDATE list_urls
		SET url_id = $1, notes = $2
		WHERE id = $3 AND deleted_at IS NULL
		RETURNING id, list_id, url_id, notes, position, created_at
	`, urlID, notesVal, id).Scan(
		&lu.ID, &lu.ListID, &lu.URLID, &notesVal, &lu.Position, &lu.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrListURLNotFound
	}
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrURLAlreadyInList
		}
		return nil, fmt.Errorf("failed to update list url: %w", err)
	}
	lu.Notes = notesVal.String
	return &lu, nil
}

// Remove soft-deletes a URL from a list
func (s *ListURLStore) Remove(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE list_urls SET deleted_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("failed to remove url from list: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check remove result: %w", err)
	}
	if rows == 0 {
		return ErrListURLNotFound
	}
	return nil
}

// Count returns the number of URLs in a list
func (s *ListURLStore) Count(ctx context.Context, listID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM list_urls
		WHERE list_id = $1 AND deleted_at IS NULL
	`, listID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count list urls: %w", err)
	}
	return count, nil
}

// isUniqueViolation checks if an error is a PostgreSQL unique constraint violation
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "duplicate key")
}
