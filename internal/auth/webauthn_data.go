package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/go-webauthn/webauthn/protocol"
)

// PasskeyRegisterData contains the data needed for client-side passkey registration
type PasskeyRegisterData struct {
	Challenge       string `json:"challenge"`
	RPID            string `json:"rpId"`
	RPName          string `json:"rpName"`
	UserID          string `json:"userId"`
	UserName        string `json:"userName"`
	UserDisplayName string `json:"userDisplayName"`
}

// JSON returns the data as a JSON string for embedding in HTML
func (d PasskeyRegisterData) JSON() string {
	b, _ := json.Marshal(d)
	return string(b)
}

// NewPasskeyRegisterData extracts registration data from WebAuthn credential creation options
func NewPasskeyRegisterData(options *protocol.CredentialCreation) (*PasskeyRegisterData, error) {
	var userIDBinary []byte

	// Handle both []byte and protocol.URLEncodedBase64 (which is a named []byte type)
	switch id := options.Response.User.ID.(type) {
	case []byte:
		userIDBinary = id
	case protocol.URLEncodedBase64:
		userIDBinary = []byte(id)
	default:
		return nil, fmt.Errorf("invalid user ID type: expected []byte or URLEncodedBase64, got %T", options.Response.User.ID)
	}

	return &PasskeyRegisterData{
		Challenge:       base64.RawURLEncoding.EncodeToString(options.Response.Challenge),
		RPID:            options.Response.RelyingParty.ID,
		RPName:          options.Response.RelyingParty.Name,
		UserID:          base64.RawURLEncoding.EncodeToString(userIDBinary),
		UserName:        options.Response.User.Name,
		UserDisplayName: options.Response.User.DisplayName,
	}, nil
}
