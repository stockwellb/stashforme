package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

var (
	// ErrPasskeyNotFound indicates the passkey doesn't exist
	ErrPasskeyNotFound = errors.New("passkey not found")
	// ErrPasskeyExists indicates a passkey with this credential ID already exists
	ErrPasskeyExists = errors.New("passkey already registered")
)

// PasskeyConfig holds WebAuthn configuration
type PasskeyConfig struct {
	RPID          string // Relying Party ID (domain)
	RPOrigin      string // Relying Party Origin (full URL)
	RPDisplayName string // Relying Party Display Name
}

// Passkey represents a stored passkey credential
type Passkey struct {
	ID           string
	UserID       string
	CredentialID []byte
	PublicKey    []byte
	SignCount    uint32
	DeviceName   string
	CreatedAt    time.Time
	LastUsedAt   *time.Time
}

// PasskeyService handles WebAuthn passkey operations
type PasskeyService struct {
	db       *sql.DB
	webauthn *webauthn.WebAuthn
	users    *UserStore
}

// NewPasskeyService creates a new passkey service
func NewPasskeyService(db *sql.DB, users *UserStore, config PasskeyConfig) (*PasskeyService, error) {
	wconfig := &webauthn.Config{
		RPID:          config.RPID,
		RPDisplayName: config.RPDisplayName,
		RPOrigins:     []string{config.RPOrigin},
	}

	w, err := webauthn.New(wconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create webauthn: %w", err)
	}

	return &PasskeyService{
		db:       db,
		webauthn: w,
		users:    users,
	}, nil
}

// webauthnUser adapts our User to the webauthn.User interface
type webauthnUser struct {
	user        *User
	credentials []webauthn.Credential
}

func (u *webauthnUser) WebAuthnID() []byte {
	return []byte(u.user.ID)
}

func (u *webauthnUser) WebAuthnName() string {
	return u.user.PhoneNumber
}

func (u *webauthnUser) WebAuthnDisplayName() string {
	if u.user.DisplayName != "" {
		return u.user.DisplayName
	}
	return u.user.PhoneNumber
}

func (u *webauthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.credentials
}

// BeginRegistration starts the passkey registration process
func (s *PasskeyService) BeginRegistration(ctx context.Context, userID string) (*protocol.CredentialCreation, []byte, error) {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, nil, err
	}

	// Get existing credentials to exclude
	credentials, err := s.getCredentialsForUser(ctx, userID)
	if err != nil {
		return nil, nil, err
	}

	wUser := &webauthnUser{user: user, credentials: credentials}

	options, session, err := s.webauthn.BeginRegistration(wUser,
		webauthn.WithExclusions(credentialDescriptors(credentials)),
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementPreferred),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin registration: %w", err)
	}

	sessionData, err := json.Marshal(session)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal session: %w", err)
	}

	return options, sessionData, nil
}

// FinishRegistration completes the passkey registration process
func (s *PasskeyService) FinishRegistration(ctx context.Context, userID string, sessionData []byte, response *protocol.ParsedCredentialCreationData) error {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}

	credentials, err := s.getCredentialsForUser(ctx, userID)
	if err != nil {
		return err
	}

	wUser := &webauthnUser{user: user, credentials: credentials}

	var session webauthn.SessionData
	if err := json.Unmarshal(sessionData, &session); err != nil {
		return fmt.Errorf("failed to unmarshal session: %w", err)
	}

	credential, err := s.webauthn.CreateCredential(wUser, session, response)
	if err != nil {
		return fmt.Errorf("failed to create credential: %w", err)
	}

	// Store the credential
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO passkeys (user_id, credential_id, public_key, sign_count, backup_eligible, backup_state)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, userID, credential.ID, credential.PublicKey, credential.Authenticator.SignCount,
		credential.Flags.BackupEligible, credential.Flags.BackupState)
	if err != nil {
		return fmt.Errorf("failed to store passkey: %w", err)
	}

	return nil
}

// BeginLogin starts the passkey authentication process
func (s *PasskeyService) BeginLogin(ctx context.Context, userID string) (*protocol.CredentialAssertion, []byte, error) {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, nil, err
	}

	credentials, err := s.getCredentialsForUser(ctx, userID)
	if err != nil {
		return nil, nil, err
	}

	if len(credentials) == 0 {
		return nil, nil, ErrPasskeyNotFound
	}

	wUser := &webauthnUser{user: user, credentials: credentials}

	options, session, err := s.webauthn.BeginLogin(wUser)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin login: %w", err)
	}

	sessionData, err := json.Marshal(session)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal session: %w", err)
	}

	return options, sessionData, nil
}

// BeginDiscoverableLogin starts a discoverable passkey login (no user ID required)
func (s *PasskeyService) BeginDiscoverableLogin(ctx context.Context) (*protocol.CredentialAssertion, []byte, error) {
	options, session, err := s.webauthn.BeginDiscoverableLogin()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin discoverable login: %w", err)
	}

	sessionData, err := json.Marshal(session)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal session: %w", err)
	}

	return options, sessionData, nil
}

// FinishLogin completes the passkey authentication process
func (s *PasskeyService) FinishLogin(ctx context.Context, userID string, sessionData []byte, response *protocol.ParsedCredentialAssertionData) error {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}

	credentials, err := s.getCredentialsForUser(ctx, userID)
	if err != nil {
		return err
	}

	wUser := &webauthnUser{user: user, credentials: credentials}

	var session webauthn.SessionData
	if err := json.Unmarshal(sessionData, &session); err != nil {
		return fmt.Errorf("failed to unmarshal session: %w", err)
	}

	credential, err := s.webauthn.ValidateLogin(wUser, session, response)
	if err != nil {
		return fmt.Errorf("failed to validate login: %w", err)
	}

	// Update sign count and last used
	_, err = s.db.ExecContext(ctx, `
		UPDATE passkeys SET sign_count = $1, last_used_at = NOW()
		WHERE credential_id = $2 AND user_id = $3
	`, credential.Authenticator.SignCount, credential.ID, userID)
	if err != nil {
		return fmt.Errorf("failed to update passkey: %w", err)
	}

	return nil
}

// FinishDiscoverableLogin completes a discoverable passkey login
func (s *PasskeyService) FinishDiscoverableLogin(ctx context.Context, sessionData []byte, response *protocol.ParsedCredentialAssertionData) (string, error) {
	var session webauthn.SessionData
	if err := json.Unmarshal(sessionData, &session); err != nil {
		return "", fmt.Errorf("failed to unmarshal session: %w", err)
	}

	// Find the user by credential ID
	var userID string
	var publicKey []byte
	var signCount uint32
	err := s.db.QueryRowContext(ctx, `
		SELECT user_id, public_key, sign_count FROM passkeys
		WHERE credential_id = $1
	`, response.RawID).Scan(&userID, &publicKey, &signCount)
	if err == sql.ErrNoRows {
		return "", ErrPasskeyNotFound
	}
	if err != nil {
		return "", fmt.Errorf("failed to find passkey: %w", err)
	}

	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return "", err
	}

	credentials, err := s.getCredentialsForUser(ctx, userID)
	if err != nil {
		return "", err
	}

	wUser := &webauthnUser{user: user, credentials: credentials}

	credential, err := s.webauthn.ValidateDiscoverableLogin(
		func(rawID, userHandle []byte) (webauthn.User, error) {
			return wUser, nil
		},
		session,
		response,
	)
	if err != nil {
		return "", fmt.Errorf("failed to validate discoverable login: %w", err)
	}

	// Update sign count and last used
	_, err = s.db.ExecContext(ctx, `
		UPDATE passkeys SET sign_count = $1, last_used_at = NOW()
		WHERE credential_id = $2
	`, credential.Authenticator.SignCount, credential.ID)
	if err != nil {
		return "", fmt.Errorf("failed to update passkey: %w", err)
	}

	return userID, nil
}

// HasPasskey checks if a user has any registered passkeys
func (s *PasskeyService) HasPasskey(ctx context.Context, userID string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM passkeys WHERE user_id = $1
	`, userID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check passkeys: %w", err)
	}
	return count > 0, nil
}

// getCredentialsForUser retrieves all credentials for a user
func (s *PasskeyService) getCredentialsForUser(ctx context.Context, userID string) ([]webauthn.Credential, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT credential_id, public_key, sign_count, backup_eligible, backup_state FROM passkeys
		WHERE user_id = $1
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials: %w", err)
	}
	defer rows.Close()

	var credentials []webauthn.Credential
	for rows.Next() {
		var credID, pubKey []byte
		var signCount uint32
		var backupEligible, backupState bool
		if err := rows.Scan(&credID, &pubKey, &signCount, &backupEligible, &backupState); err != nil {
			return nil, fmt.Errorf("failed to scan credential: %w", err)
		}
		credentials = append(credentials, webauthn.Credential{
			ID:        credID,
			PublicKey: pubKey,
			Flags: webauthn.CredentialFlags{
				BackupEligible: backupEligible,
				BackupState:    backupState,
			},
			Authenticator: webauthn.Authenticator{
				SignCount: signCount,
			},
		})
	}

	return credentials, rows.Err()
}

// credentialDescriptors converts credentials to protocol descriptors for exclusion
func credentialDescriptors(credentials []webauthn.Credential) []protocol.CredentialDescriptor {
	descriptors := make([]protocol.CredentialDescriptor, len(credentials))
	for i, cred := range credentials {
		descriptors[i] = protocol.CredentialDescriptor{
			Type:         protocol.PublicKeyCredentialType,
			CredentialID: cred.ID,
		}
	}
	return descriptors
}
