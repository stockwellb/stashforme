package views

import "encoding/json"

type PasskeyRegisterData struct {
	Challenge       string `json:"challenge"`
	RPID            string `json:"rpId"`
	RPName          string `json:"rpName"`
	UserID          string `json:"userId"`
	UserName        string `json:"userName"`
	UserDisplayName string `json:"userDisplayName"`
}

func (d PasskeyRegisterData) JSON() string {
	b, _ := json.Marshal(d)
	return string(b)
}
