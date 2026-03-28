package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

const (
	// SessionTokenLength is the length of the raw session token in bytes
	SessionTokenLength = 32
	// SessionExpiry is how long a session is valid
	SessionExpiry = 30 * 24 * time.Hour // 30 days
	// SessionCookieName is the name of the session cookie
	SessionCookieName = "session"
)

var (
	// ErrSessionNotFound indicates the session doesn't exist or is expired
	ErrSessionNotFound = errors.New("session not found or expired")
	// ErrSessionExpired indicates the session has expired
	ErrSessionExpired = errors.New("session has expired")
)

// Session represents a user session
type Session struct {
	ID           string
	UserID       string
	TokenHash    string
	UserAgent    string
	IPAddress    string
	ExpiresAt    time.Time
	CreatedAt    time.Time
	LastActiveAt time.Time
}

// SessionStore handles session persistence
type SessionStore struct {
	db *sql.DB
}

// NewSessionStore creates a new session store
func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db}
}

// GenerateToken creates a cryptographically secure session token
func GenerateToken() (string, error) {
	bytes := make([]byte, SessionTokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate session token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// HashToken creates a SHA-256 hash of the token for storage
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// Create creates a new session for the user and returns the raw token
func (s *SessionStore) Create(ctx context.Context, userID, userAgent, ipAddress string) (string, error) {
	token, err := GenerateToken()
	if err != nil {
		return "", err
	}

	tokenHash := HashToken(token)
	expiresAt := time.Now().Add(SessionExpiry)

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO sessions (user_id, token_hash, user_agent, ip_address, expires_at)
		VALUES ($1, $2, $3, $4, $5)
	`, userID, tokenHash, userAgent, ipAddress, expiresAt)
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	return token, nil
}

// Validate checks if a session token is valid and returns the user ID
func (s *SessionStore) Validate(ctx context.Context, token string) (string, error) {
	tokenHash := HashToken(token)

	var userID string
	var expiresAt time.Time

	err := s.db.QueryRowContext(ctx, `
		SELECT user_id, expires_at FROM sessions
		WHERE token_hash = $1
	`, tokenHash).Scan(&userID, &expiresAt)
	if err == sql.ErrNoRows {
		return "", ErrSessionNotFound
	}
	if err != nil {
		return "", fmt.Errorf("failed to validate session: %w", err)
	}

	if time.Now().After(expiresAt) {
		return "", ErrSessionExpired
	}

	// Update last active time
	_, _ = s.db.ExecContext(ctx, `
		UPDATE sessions SET last_active_at = NOW() WHERE token_hash = $1
	`, tokenHash)

	return userID, nil
}

// Delete removes a session by token
func (s *SessionStore) Delete(ctx context.Context, token string) error {
	tokenHash := HashToken(token)

	_, err := s.db.ExecContext(ctx, `
		DELETE FROM sessions WHERE token_hash = $1
	`, tokenHash)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	return nil
}

// DeleteAllForUser removes all sessions for a user
func (s *SessionStore) DeleteAllForUser(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM sessions WHERE user_id = $1
	`, userID)
	if err != nil {
		return fmt.Errorf("failed to delete user sessions: %w", err)
	}

	return nil
}

// CleanupExpired removes all expired sessions
func (s *SessionStore) CleanupExpired(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM sessions WHERE expires_at < NOW()
	`)
	if err != nil {
		return fmt.Errorf("failed to cleanup expired sessions: %w", err)
	}

	return nil
}
