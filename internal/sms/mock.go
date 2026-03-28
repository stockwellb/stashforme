package sms

import (
	"context"
	"log"
)

// MockProvider implements SMS delivery by logging to console (for development)
type MockProvider struct{}

// NewMockProvider creates a new mock SMS provider
func NewMockProvider() *MockProvider {
	return &MockProvider{}
}

// Send logs the SMS message to the console instead of sending
func (p *MockProvider) Send(ctx context.Context, to, message string) error {
	log.Printf("[MOCK SMS] To: %s | Message: %s", to, message)
	return nil
}
