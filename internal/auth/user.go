package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrUserNotFound indicates the user doesn't exist
	ErrUserNotFound = errors.New("user not found")
)

// User represents a user in the system
type User struct {
	ID          string
	PhoneNumber string
	DisplayName string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// UserStore handles user persistence
type UserStore struct {
	db *sql.DB
}

// NewUserStore creates a new user store
func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{db: db}
}

// FindByPhone finds a user by phone number
func (s *UserStore) FindByPhone(ctx context.Context, phone string) (*User, error) {
	if err := ValidatePhoneNumber(phone); err != nil {
		return nil, err
	}

	var user User
	var displayName sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, phone_number, display_name, created_at, updated_at
		FROM users
		WHERE phone_number = $1 AND deleted_at IS NULL
	`, phone).Scan(&user.ID, &user.PhoneNumber, &displayName, &user.CreatedAt, &user.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find user: %w", err)
	}
	user.DisplayName = displayName.String

	return &user, nil
}

// FindByID finds a user by ID
func (s *UserStore) FindByID(ctx context.Context, id string) (*User, error) {
	var user User
	var displayName sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, phone_number, display_name, created_at, updated_at
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
	`, id).Scan(&user.ID, &user.PhoneNumber, &displayName, &user.CreatedAt, &user.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find user: %w", err)
	}
	user.DisplayName = displayName.String

	return &user, nil
}

// FindOrCreate finds an existing user by phone or creates a new one
func (s *UserStore) FindOrCreate(ctx context.Context, phone string) (*User, bool, error) {
	if err := ValidatePhoneNumber(phone); err != nil {
		return nil, false, err
	}

	// Try to find existing user
	user, err := s.FindByPhone(ctx, phone)
	if err == nil {
		return user, false, nil
	}
	if !errors.Is(err, ErrUserNotFound) {
		return nil, false, err
	}

	// Create new user
	var id string
	var createdAt, updatedAt time.Time
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO users (phone_number)
		VALUES ($1)
		RETURNING id, created_at, updated_at
	`, phone).Scan(&id, &createdAt, &updatedAt)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create user: %w", err)
	}

	return &User{
		ID:          id,
		PhoneNumber: phone,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}, true, nil
}

// UpdateDisplayName updates the user's display name
func (s *UserStore) UpdateDisplayName(ctx context.Context, id, displayName string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE users SET display_name = $1, updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`, displayName, id)
	if err != nil {
		return fmt.Errorf("failed to update display name: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check update result: %w", err)
	}
	if rows == 0 {
		return ErrUserNotFound
	}

	return nil
}
