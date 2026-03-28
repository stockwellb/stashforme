package stash

import (
	"context"
	"database/sql"
	"fmt"
)

// ListStore handles list persistence
type ListStore struct {
	db *sql.DB
}

// NewListStore creates a new list store
func NewListStore(db *sql.DB) *ListStore {
	return &ListStore{db: db}
}

// FindByID finds a list by ID (must belong to user)
func (s *ListStore) FindByID(ctx context.Context, id, userID string) (*List, error) {
	var list List
	var description sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, name, description, is_default, position, created_at, updated_at
		FROM lists
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`, id, userID).Scan(
		&list.ID, &list.UserID, &list.Name, &description,
		&list.IsDefault, &list.Position, &list.CreatedAt, &list.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrListNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find list: %w", err)
	}
	list.Description = description.String
	return &list, nil
}

// FindByUserID finds all lists for a user
func (s *ListStore) FindByUserID(ctx context.Context, userID string) ([]*List, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, name, description, is_default, position, created_at, updated_at
		FROM lists
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY is_default DESC, position ASC, created_at ASC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to find lists: %w", err)
	}
	defer rows.Close()

	var lists []*List
	for rows.Next() {
		var list List
		var description sql.NullString
		if err := rows.Scan(
			&list.ID, &list.UserID, &list.Name, &description,
			&list.IsDefault, &list.Position, &list.CreatedAt, &list.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan list: %w", err)
		}
		list.Description = description.String
		lists = append(lists, &list)
	}
	return lists, rows.Err()
}

// FindDefaultByUserID finds the default list for a user
func (s *ListStore) FindDefaultByUserID(ctx context.Context, userID string) (*List, error) {
	var list List
	var description sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, name, description, is_default, position, created_at, updated_at
		FROM lists
		WHERE user_id = $1 AND is_default = TRUE AND deleted_at IS NULL
	`, userID).Scan(
		&list.ID, &list.UserID, &list.Name, &description,
		&list.IsDefault, &list.Position, &list.CreatedAt, &list.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrListNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find default list: %w", err)
	}
	list.Description = description.String
	return &list, nil
}

// Create creates a new list
func (s *ListStore) Create(ctx context.Context, userID, name, description string) (*List, error) {
	var list List
	var descVal sql.NullString
	if description != "" {
		descVal = sql.NullString{String: description, Valid: true}
	}

	err := s.db.QueryRowContext(ctx, `
		INSERT INTO lists (user_id, name, description, position)
		VALUES ($1, $2, $3, (SELECT COALESCE(MAX(position), 0) + 1 FROM lists WHERE user_id = $1 AND deleted_at IS NULL))
		RETURNING id, user_id, name, description, is_default, position, created_at, updated_at
	`, userID, name, descVal).Scan(
		&list.ID, &list.UserID, &list.Name, &descVal,
		&list.IsDefault, &list.Position, &list.CreatedAt, &list.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create list: %w", err)
	}
	list.Description = descVal.String
	return &list, nil
}

// CreateDefault creates the default list for a user
func (s *ListStore) CreateDefault(ctx context.Context, userID string) (*List, error) {
	var list List
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO lists (user_id, name, is_default, position)
		VALUES ($1, 'Inbox', TRUE, 0)
		ON CONFLICT DO NOTHING
		RETURNING id, user_id, name, description, is_default, position, created_at, updated_at
	`, userID).Scan(
		&list.ID, &list.UserID, &list.Name, &sql.NullString{},
		&list.IsDefault, &list.Position, &list.CreatedAt, &list.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		// Already exists, fetch it
		return s.FindDefaultByUserID(ctx, userID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create default list: %w", err)
	}
	return &list, nil
}

// FindOrCreateDefault finds or creates the default list for a user
func (s *ListStore) FindOrCreateDefault(ctx context.Context, userID string) (*List, error) {
	list, err := s.FindDefaultByUserID(ctx, userID)
	if err == nil {
		return list, nil
	}
	if err != ErrListNotFound {
		return nil, err
	}
	return s.CreateDefault(ctx, userID)
}

// Update updates a list's name and description
func (s *ListStore) Update(ctx context.Context, id, userID, name, description string) (*List, error) {
	var list List
	var descVal sql.NullString
	if description != "" {
		descVal = sql.NullString{String: description, Valid: true}
	}

	err := s.db.QueryRowContext(ctx, `
		UPDATE lists
		SET name = $1, description = $2, updated_at = NOW()
		WHERE id = $3 AND user_id = $4 AND deleted_at IS NULL
		RETURNING id, user_id, name, description, is_default, position, created_at, updated_at
	`, name, descVal, id, userID).Scan(
		&list.ID, &list.UserID, &list.Name, &descVal,
		&list.IsDefault, &list.Position, &list.CreatedAt, &list.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrListNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to update list: %w", err)
	}
	list.Description = descVal.String
	return &list, nil
}

// Delete soft-deletes a list (cannot delete default list)
func (s *ListStore) Delete(ctx context.Context, id, userID string) error {
	// Check if it's the default list
	list, err := s.FindByID(ctx, id, userID)
	if err != nil {
		return err
	}
	if list.IsDefault {
		return ErrCannotDeleteDefaultList
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE lists SET deleted_at = NOW()
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`, id, userID)
	if err != nil {
		return fmt.Errorf("failed to delete list: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check delete result: %w", err)
	}
	if rows == 0 {
		return ErrListNotFound
	}
	return nil
}
