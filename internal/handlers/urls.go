package handlers

import (
	"net/url"
)

// Common route paths
const (
	PathLogin        = "/login"
	PathVerify       = "/verify"
	PathMyStash      = "/my/stash"
	PathMyAccount    = "/my/account"
	PathMe           = "/me"
	PathPasskeySetup = "/passkey/setup"

	// Stash routes
	PathStashLists    = "/my/stash/lists"
	PathStashListURLs = "/my/stash/lists/:id/urls"
	PathStashURLs     = "/my/stash/urls"
)

// BuildURL constructs a URL with query parameters
func BuildURL(path string, params map[string]string) string {
	if len(params) == 0 {
		return path
	}

	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}
	return path + "?" + values.Encode()
}

// LoginURL returns the login page URL with optional error
func LoginURL(errorMsg string) string {
	if errorMsg == "" {
		return PathLogin
	}
	return BuildURL(PathLogin, map[string]string{"error": errorMsg})
}

// VerifyURL returns the verify page URL
func VerifyURL(phone, errorMsg string) string {
	params := map[string]string{"phone": phone}
	if errorMsg != "" {
		params["error"] = errorMsg
	}
	return BuildURL(PathVerify, params)
}
