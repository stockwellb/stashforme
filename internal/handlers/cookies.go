package handlers

import (
	"encoding/base64"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"stashforme/internal/auth"
)

const (
	webauthnSessionCookie = "webauthn_session"
	pendingUserIDCookie   = "pending_user_id"
	passkeyOptionsCookie  = "passkey_options"
	webauthnCookieMaxAge  = 300 // 5 minutes
)

// setSessionCookie sets the session cookie for authenticated users
func setSessionCookie(c echo.Context, token string) {
	c.SetCookie(&http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   c.Request().TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(auth.SessionExpiry.Seconds()),
	})
}

// setWebAuthnSessionCookie stores WebAuthn session data
func setWebAuthnSessionCookie(c echo.Context, sessionData []byte) {
	c.SetCookie(&http.Cookie{
		Name:     webauthnSessionCookie,
		Value:    base64.StdEncoding.EncodeToString(sessionData),
		Path:     "/",
		HttpOnly: true,
		Secure:   c.Request().TLS != nil,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   webauthnCookieMaxAge,
	})
}

// setPendingUserCookie stores the pending user ID during WebAuthn flow
func setPendingUserCookie(c echo.Context, userID string) {
	c.SetCookie(&http.Cookie{
		Name:     pendingUserIDCookie,
		Value:    userID,
		Path:     "/",
		HttpOnly: true,
		Secure:   c.Request().TLS != nil,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   webauthnCookieMaxAge,
	})
}

// setPasskeyOptionsCookie stores passkey registration options for JS
func setPasskeyOptionsCookie(c echo.Context, data []byte) {
	c.SetCookie(&http.Cookie{
		Name:     passkeyOptionsCookie,
		Value:    base64.StdEncoding.EncodeToString(data),
		Path:     "/",
		HttpOnly: false, // JS needs to read this
		Secure:   c.Request().TLS != nil,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   webauthnCookieMaxAge,
	})
}

// getWebAuthnSessionData retrieves and decodes WebAuthn session data
func getWebAuthnSessionData(c echo.Context) ([]byte, error) {
	cookie, err := c.Cookie(webauthnSessionCookie)
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(cookie.Value)
}

// getPendingUserID retrieves the pending user ID
func getPendingUserID(c echo.Context) (string, error) {
	cookie, err := c.Cookie(pendingUserIDCookie)
	if err != nil {
		return "", err
	}
	return cookie.Value, nil
}

// clearWebAuthnCookies removes all WebAuthn-related cookies
func clearWebAuthnCookies(c echo.Context) {
	for _, name := range []string{webauthnSessionCookie, pendingUserIDCookie, passkeyOptionsCookie} {
		c.SetCookie(&http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   c.Request().TLS != nil,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   -1,
			Expires:  time.Unix(0, 0),
		})
	}
}

// clearSessionCookie removes the session cookie
func clearSessionCookie(c echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   c.Request().TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}
