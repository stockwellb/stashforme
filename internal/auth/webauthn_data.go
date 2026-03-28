package auth

import "encoding/json"

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
