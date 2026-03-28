package sms

import "context"

// Provider defines the interface for SMS delivery services
type Provider interface {
	// Send delivers an SMS message to the specified phone number
	Send(ctx context.Context, to, message string) error
}
