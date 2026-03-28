package handlers

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/labstack/echo/v4"

	"stashforme/internal/auth"
	"stashforme/internal/views"
)

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	otp      *auth.OTPService
	sessions *auth.SessionStore
	users    *auth.UserStore
	passkeys *auth.PasskeyService
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(
	otp *auth.OTPService,
	sessions *auth.SessionStore,
	users *auth.UserStore,
	passkeys *auth.PasskeyService,
) *AuthHandler {
	return &AuthHandler{
		otp:      otp,
		sessions: sessions,
		users:    users,
		passkeys: passkeys,
	}
}

// Login renders the login page
func (h *AuthHandler) Login(c echo.Context) error {
	return Render(c, http.StatusOK, views.Login())
}

// SendCode sends an OTP code to the phone number
func (h *AuthHandler) SendCode(c echo.Context) error {
	phone := c.FormValue("phone")
	if phone == "" {
		return Render(c, http.StatusBadRequest, views.AuthError("Phone number is required"))
	}

	// Normalize phone number (add + and country code if missing)
	phone = auth.NormalizePhoneNumber(phone)

	if err := h.otp.SendCode(c.Request().Context(), phone); err != nil {
		if errors.Is(err, auth.ErrInvalidPhoneNumber) {
			return Render(c, http.StatusBadRequest, views.AuthError(err.Error()))
		}
		if errors.Is(err, auth.ErrRateLimited) {
			return Render(c, http.StatusTooManyRequests, views.AuthError(err.Error()))
		}
		c.Logger().Error("Failed to send OTP:", err)
		return Render(c, http.StatusInternalServerError, views.AuthError("Failed to send verification code"))
	}

	return Render(c, http.StatusOK, views.VerifyCodeForm(phone))
}

// VerifyCode verifies the OTP code and starts passkey registration
func (h *AuthHandler) VerifyCode(c echo.Context) error {
	phone := c.FormValue("phone")
	code := c.FormValue("code")

	if phone == "" || code == "" {
		return Render(c, http.StatusBadRequest, views.AuthError("Phone number and code are required"))
	}

	// Normalize phone number
	phone = auth.NormalizePhoneNumber(phone)

	if err := h.otp.VerifyCode(c.Request().Context(), phone, code); err != nil {
		if errors.Is(err, auth.ErrOTPInvalid) || errors.Is(err, auth.ErrOTPExpired) || errors.Is(err, auth.ErrOTPMaxAttempts) {
			return Render(c, http.StatusBadRequest, views.AuthError(err.Error()))
		}
		c.Logger().Error("Failed to verify OTP:", err)
		return Render(c, http.StatusInternalServerError, views.AuthError("Failed to verify code"))
	}

	// Find or create user
	user, _, err := h.users.FindOrCreate(c.Request().Context(), phone)
	if err != nil {
		c.Logger().Error("Failed to find/create user:", err)
		return Render(c, http.StatusInternalServerError, views.AuthError("Failed to create account"))
	}

	// Check if user has passkeys
	hasPasskey, err := h.passkeys.HasPasskey(c.Request().Context(), user.ID)
	if err != nil {
		c.Logger().Error("Failed to check passkeys:", err)
		return Render(c, http.StatusInternalServerError, views.AuthError("Failed to check passkeys"))
	}

	if hasPasskey {
		// User has passkeys, create session directly (they verified via SMS)
		return h.createSessionAndRespond(c, user.ID)
	}

	// Start passkey registration
	options, sessionData, err := h.passkeys.BeginRegistration(c.Request().Context(), user.ID)
	if err != nil {
		c.Logger().Error("Failed to begin registration:", err)
		return Render(c, http.StatusInternalServerError, views.AuthError("Failed to start passkey registration"))
	}

	// Store session data in cookie for later
	c.SetCookie(&http.Cookie{
		Name:     "webauthn_session",
		Value:    base64.StdEncoding.EncodeToString(sessionData),
		Path:     "/",
		HttpOnly: true,
		Secure:   c.Request().TLS != nil,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   300, // 5 minutes
	})

	// Store user ID for registration completion
	c.SetCookie(&http.Cookie{
		Name:     "pending_user_id",
		Value:    user.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   c.Request().TLS != nil,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   300,
	})

	// Prepare data for template with base64-encoded values
	userID, _ := options.Response.User.ID.([]byte)
	data := views.PasskeyRegisterData{
		Challenge:       base64.RawURLEncoding.EncodeToString(options.Response.Challenge),
		RPID:            options.Response.RelyingParty.ID,
		RPName:          options.Response.RelyingParty.Name,
		UserID:          base64.RawURLEncoding.EncodeToString(userID),
		UserName:        options.Response.User.Name,
		UserDisplayName: options.Response.User.DisplayName,
	}

	return Render(c, http.StatusOK, views.PasskeyRegister(data))
}

// PasskeyRegister completes passkey registration
func (h *AuthHandler) PasskeyRegister(c echo.Context) error {
	sessionCookie, err := c.Cookie("webauthn_session")
	if err != nil {
		return Render(c, http.StatusBadRequest, views.AuthError("Registration session expired"))
	}

	userIDCookie, err := c.Cookie("pending_user_id")
	if err != nil {
		return Render(c, http.StatusBadRequest, views.AuthError("Registration session expired"))
	}

	sessionData, err := base64.StdEncoding.DecodeString(sessionCookie.Value)
	if err != nil {
		return Render(c, http.StatusBadRequest, views.AuthError("Invalid session data"))
	}

	// Parse the credential from request body
	var credentialResponse protocol.CredentialCreationResponse
	if err := json.NewDecoder(c.Request().Body).Decode(&credentialResponse); err != nil {
		return Render(c, http.StatusBadRequest, views.AuthError("Invalid credential data"))
	}

	parsedResponse, err := credentialResponse.Parse()
	if err != nil {
		return Render(c, http.StatusBadRequest, views.AuthError("Failed to parse credential"))
	}

	if err := h.passkeys.FinishRegistration(c.Request().Context(), userIDCookie.Value, sessionData, parsedResponse); err != nil {
		c.Logger().Error("Failed to finish registration:", err)
		return Render(c, http.StatusInternalServerError, views.AuthError("Failed to register passkey"))
	}

	// Clear WebAuthn cookies
	h.clearWebAuthnCookies(c)

	// Create session
	return h.createSessionAndRespond(c, userIDCookie.Value)
}

// PasskeyLogin starts passkey authentication
func (h *AuthHandler) PasskeyLogin(c echo.Context) error {
	phone := c.FormValue("phone")
	if phone == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Phone number is required"})
	}

	phone = auth.NormalizePhoneNumber(phone)

	// Find user by phone
	user, err := h.users.FindByPhone(c.Request().Context(), phone)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "No account found with this phone number"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to find user"})
	}

	// Start login for this user
	options, sessionData, err := h.passkeys.BeginLogin(c.Request().Context(), user.ID)
	if err != nil {
		if errors.Is(err, auth.ErrPasskeyNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "No passkey registered for this account"})
		}
		c.Logger().Error("Failed to begin login:", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to start login"})
	}

	// Store session data and user ID in cookies
	c.SetCookie(&http.Cookie{
		Name:     "webauthn_session",
		Value:    base64.StdEncoding.EncodeToString(sessionData),
		Path:     "/",
		HttpOnly: true,
		Secure:   c.Request().TLS != nil,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   300,
	})
	c.SetCookie(&http.Cookie{
		Name:     "pending_user_id",
		Value:    user.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   c.Request().TLS != nil,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   300,
	})

	return c.JSON(http.StatusOK, options)
}

// PasskeyLoginFinish completes passkey authentication
func (h *AuthHandler) PasskeyLoginFinish(c echo.Context) error {
	sessionCookie, err := c.Cookie("webauthn_session")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Login session expired"})
	}

	userIDCookie, err := c.Cookie("pending_user_id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Login session expired"})
	}

	sessionData, err := base64.StdEncoding.DecodeString(sessionCookie.Value)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid session data"})
	}

	// Parse the assertion from request body
	var assertionResponse protocol.CredentialAssertionResponse
	if err := json.NewDecoder(c.Request().Body).Decode(&assertionResponse); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid assertion data"})
	}

	parsedResponse, err := assertionResponse.Parse()
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Failed to parse assertion"})
	}

	userID := userIDCookie.Value
	if err := h.passkeys.FinishLogin(c.Request().Context(), userID, sessionData, parsedResponse); err != nil {
		if errors.Is(err, auth.ErrPasskeyNotFound) {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Passkey not found"})
		}
		c.Logger().Error("Failed to finish login:", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to verify passkey"})
	}

	// Clear WebAuthn cookies
	h.clearWebAuthnCookies(c)

	// Create session
	token, err := h.sessions.Create(
		c.Request().Context(),
		userID,
		c.Request().UserAgent(),
		c.RealIP(),
	)
	if err != nil {
		c.Logger().Error("Failed to create session:", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create session"})
	}

	h.setSessionCookie(c, token)

	return c.JSON(http.StatusOK, map[string]string{"redirect": "/"})
}

// Logout destroys the current session
func (h *AuthHandler) Logout(c echo.Context) error {
	cookie, err := c.Cookie(auth.SessionCookieName)
	if err == nil {
		_ = h.sessions.Delete(c.Request().Context(), cookie.Value)
	}

	// Clear session cookie
	c.SetCookie(&http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   c.Request().TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	// For HTMX requests, redirect via HX-Redirect header
	if c.Request().Header.Get("HX-Request") == "true" {
		c.Response().Header().Set("HX-Redirect", "/login")
		return c.NoContent(http.StatusOK)
	}

	return c.Redirect(http.StatusSeeOther, "/login")
}

// createSessionAndRespond creates a session and returns a redirect response
func (h *AuthHandler) createSessionAndRespond(c echo.Context, userID string) error {
	token, err := h.sessions.Create(
		c.Request().Context(),
		userID,
		c.Request().UserAgent(),
		c.RealIP(),
	)
	if err != nil {
		c.Logger().Error("Failed to create session:", err)
		return Render(c, http.StatusInternalServerError, views.AuthError("Failed to create session"))
	}

	h.setSessionCookie(c, token)

	// For HTMX requests, redirect via HX-Redirect header
	c.Response().Header().Set("HX-Redirect", "/")
	return c.NoContent(http.StatusOK)
}

func (h *AuthHandler) setSessionCookie(c echo.Context, token string) {
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

func (h *AuthHandler) clearWebAuthnCookies(c echo.Context) {
	for _, name := range []string{"webauthn_session", "pending_user_id"} {
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
