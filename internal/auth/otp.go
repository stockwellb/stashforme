package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"time"

	"stashforme/internal/sms"
)

const (
	// OTPLength is the number of digits in an OTP code
	OTPLength = 6
	// OTPExpiry is how long an OTP is valid
	OTPExpiry = 10 * time.Minute
	// MaxOTPAttempts is the maximum verification attempts allowed
	MaxOTPAttempts = 3
	// RateLimitWindow is the time window for rate limiting
	RateLimitWindow = 1 * time.Hour
	// MaxCodesPerWindow is the maximum OTP codes per rate limit window
	MaxCodesPerWindow = 3
)

var (
	// ErrOTPExpired indicates the OTP code has expired
	ErrOTPExpired = errors.New("verification code has expired")
	// ErrOTPInvalid indicates the OTP code is incorrect
	ErrOTPInvalid = errors.New("invalid verification code")
	// ErrOTPMaxAttempts indicates too many failed verification attempts
	ErrOTPMaxAttempts = errors.New("too many failed attempts, please request a new code")
	// ErrRateLimited indicates the user has requested too many codes
	ErrRateLimited = errors.New("too many verification requests, please try again later")
)

// OTPService handles OTP generation and verification
type OTPService struct {
	db       *sql.DB
	sms      sms.Provider
}

// NewOTPService creates a new OTP service
func NewOTPService(db *sql.DB, smsProvider sms.Provider) *OTPService {
	return &OTPService{
		db:  db,
		sms: smsProvider,
	}
}

// GenerateCode creates a cryptographically secure 6-digit OTP
func GenerateCode() (string, error) {
	max := big.NewInt(1000000) // 10^6
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", fmt.Errorf("failed to generate OTP: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// SendCode generates and sends a new OTP code to the phone number
func (s *OTPService) SendCode(ctx context.Context, phone string) error {
	if err := ValidatePhoneNumber(phone); err != nil {
		return err
	}

	// Check rate limit
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM verification_codes
		WHERE phone_number = $1 AND created_at > $2
	`, phone, time.Now().Add(-RateLimitWindow)).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check rate limit: %w", err)
	}
	if count >= MaxCodesPerWindow {
		return ErrRateLimited
	}

	// Generate OTP
	code, err := GenerateCode()
	if err != nil {
		return err
	}

	// Store in database
	expiresAt := time.Now().Add(OTPExpiry)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO verification_codes (phone_number, code, expires_at)
		VALUES ($1, $2, $3)
	`, phone, code, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to store verification code: %w", err)
	}

	// Send SMS
	message := fmt.Sprintf("Your StashForMe verification code is: %s", code)
	if err := s.sms.Send(ctx, phone, message); err != nil {
		return fmt.Errorf("failed to send SMS: %w", err)
	}

	return nil
}

// VerifyCode checks if the provided code is valid for the phone number
func (s *OTPService) VerifyCode(ctx context.Context, phone, code string) error {
	if err := ValidatePhoneNumber(phone); err != nil {
		return err
	}

	var id string
	var storedCode string
	var attempts int
	var expiresAt time.Time

	err := s.db.QueryRowContext(ctx, `
		SELECT id, code, attempts, expires_at FROM verification_codes
		WHERE phone_number = $1 AND verified_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1
	`, phone).Scan(&id, &storedCode, &attempts, &expiresAt)
	if err == sql.ErrNoRows {
		return ErrOTPInvalid
	}
	if err != nil {
		return fmt.Errorf("failed to lookup verification code: %w", err)
	}

	// Check expiry
	if time.Now().After(expiresAt) {
		return ErrOTPExpired
	}

	// Check attempts
	if attempts >= MaxOTPAttempts {
		return ErrOTPMaxAttempts
	}

	// Verify code
	if code != storedCode {
		// Increment attempts
		_, _ = s.db.ExecContext(ctx, `
			UPDATE verification_codes SET attempts = attempts + 1 WHERE id = $1
		`, id)
		return ErrOTPInvalid
	}

	// Mark as verified
	_, err = s.db.ExecContext(ctx, `
		UPDATE verification_codes SET verified_at = NOW() WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("failed to mark code as verified: %w", err)
	}

	return nil
}
