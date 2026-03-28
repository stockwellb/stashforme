package sms

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// TwilioConfig holds Twilio API configuration
type TwilioConfig struct {
	AccountSID string
	AuthToken  string
	FromNumber string
}

// TwilioProvider implements SMS delivery via Twilio
type TwilioProvider struct {
	config TwilioConfig
	client *http.Client
}

// NewTwilioProvider creates a new Twilio SMS provider
func NewTwilioProvider(config TwilioConfig) *TwilioProvider {
	return &TwilioProvider{
		config: config,
		client: &http.Client{},
	}
}

// Send delivers an SMS message via Twilio
func (p *TwilioProvider) Send(ctx context.Context, to, message string) error {
	apiURL := fmt.Sprintf(
		"https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json",
		p.config.AccountSID,
	)

	data := url.Values{}
	data.Set("To", to)
	data.Set("From", p.config.FromNumber)
	data.Set("Body", message)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(p.config.AccountSID, p.config.AuthToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send SMS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errResp struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			return fmt.Errorf("twilio error: status %d", resp.StatusCode)
		}
		return fmt.Errorf("twilio error: %s (code: %d)", errResp.Message, errResp.Code)
	}

	return nil
}
