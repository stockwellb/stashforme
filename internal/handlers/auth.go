package handlers

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
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
	errorMsg := c.QueryParam("error")
	return Render(c, http.StatusOK, views.Login(errorMsg))
}

// SendCode sends an OTP code and redirects to verify page
func (h *AuthHandler) SendCode(c echo.Context) error {
	phone := c.FormValue("phone")
	if phone == "" {
		return Render(c, http.StatusBadRequest, views.Login("Phone number is required"))
	}

	phone = auth.NormalizePhoneNumber(phone)

	if err := h.otp.SendCode(c.Request().Context(), phone); err != nil {
		errorMsg := "Failed to send code"
		if errors.Is(err, auth.ErrRateLimited) {
			errorMsg = "Too many requests, please try again later"
		}
		c.Logger().Error("Failed to send code:", err)
		return c.Redirect(http.StatusSeeOther, "/login?error="+url.QueryEscape(errorMsg))
	}

	// Redirect to verify page with phone in query
	return c.Redirect(http.StatusSeeOther, "/verify?phone="+url.QueryEscape(phone))
}

// VerifyPage renders the verification code entry page
func (h *AuthHandler) VerifyPage(c echo.Context) error {
	phone := c.QueryParam("phone")
	errorMsg := c.QueryParam("error")
	if phone == "" {
		return c.Redirect(http.StatusSeeOther, "/login")
	}
	return Render(c, http.StatusOK, views.Verify(phone, errorMsg))
}

// VerifyCode verifies the OTP and redirects appropriately
func (h *AuthHandler) VerifyCode(c echo.Context) error {
	phone := c.FormValue("phone")
	code := c.FormValue("code")

	if phone == "" || code == "" {
		return c.Redirect(http.StatusSeeOther, "/login")
	}

	phone = auth.NormalizePhoneNumber(phone)

	if err := h.otp.VerifyCode(c.Request().Context(), phone, code); err != nil {
		errorMsg := "Invalid code"
		if errors.Is(err, auth.ErrOTPExpired) {
			errorMsg = "Code expired"
		} else if errors.Is(err, auth.ErrOTPMaxAttempts) {
			errorMsg = "Too many attempts"
		}
		return c.Redirect(http.StatusSeeOther, "/verify?phone="+url.QueryEscape(phone)+"&error="+url.QueryEscape(errorMsg))
	}

	// Find or create user
	user, _, err := h.users.FindOrCreate(c.Request().Context(), phone)
	if err != nil {
		c.Logger().Error("Failed to find/create user:", err)
		return c.Redirect(http.StatusSeeOther, "/verify?phone="+url.QueryEscape(phone)+"&error="+url.QueryEscape("Something went wrong"))
	}

	// Check if user has passkeys
	hasPasskey, err := h.passkeys.HasPasskey(c.Request().Context(), user.ID)
	if err != nil {
		c.Logger().Error("Failed to check passkeys:", err)
		return c.Redirect(http.StatusSeeOther, "/verify?phone="+url.QueryEscape(phone)+"&error="+url.QueryEscape("Something went wrong"))
	}

	if hasPasskey {
		// User has passkeys, create session and go to profile
		token, err := h.sessions.Create(c.Request().Context(), user.ID, c.Request().UserAgent(), c.RealIP())
		if err != nil {
			c.Logger().Error("Failed to create session:", err)
			return c.Redirect(http.StatusSeeOther, "/login")
		}
		h.setSessionCookie(c, token)
		return c.Redirect(http.StatusSeeOther, "/my/stash")
	}

	// No passkey - redirect to passkey setup
	// First, prepare the registration
	options, sessionData, err := h.passkeys.BeginRegistration(c.Request().Context(), user.ID)
	if err != nil {
		c.Logger().Error("Failed to begin registration:", err)
		// Create session anyway and skip passkey
		token, _ := h.sessions.Create(c.Request().Context(), user.ID, c.Request().UserAgent(), c.RealIP())
		h.setSessionCookie(c, token)
		return c.Redirect(http.StatusSeeOther, "/my/stash")
	}

	// Store session data in cookies
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

	// Store options data for the page
	userID, _ := options.Response.User.ID.([]byte)
	c.SetCookie(&http.Cookie{
		Name:     "passkey_options",
		Value:    base64.StdEncoding.EncodeToString(mustJSON(views.PasskeyRegisterData{
			Challenge:       base64.RawURLEncoding.EncodeToString(options.Response.Challenge),
			RPID:            options.Response.RelyingParty.ID,
			RPName:          options.Response.RelyingParty.Name,
			UserID:          base64.RawURLEncoding.EncodeToString(userID),
			UserName:        options.Response.User.Name,
			UserDisplayName: options.Response.User.DisplayName,
		})),
		Path:     "/",
		HttpOnly: false, // JS needs to read this
		Secure:   c.Request().TLS != nil,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   300,
	})

	return c.Redirect(http.StatusSeeOther, "/passkey/setup")
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// PasskeySetupPage renders the passkey setup page
func (h *AuthHandler) PasskeySetupPage(c echo.Context) error {
	// Get options from cookie
	optionsCookie, err := c.Cookie("passkey_options")
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/login")
	}

	optionsData, err := base64.StdEncoding.DecodeString(optionsCookie.Value)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/login")
	}

	var data views.PasskeyRegisterData
	if err := json.Unmarshal(optionsData, &data); err != nil {
		return c.Redirect(http.StatusSeeOther, "/login")
	}

	return Render(c, http.StatusOK, views.PasskeySetup(data))
}

// PasskeyRegister completes passkey registration
func (h *AuthHandler) PasskeyRegister(c echo.Context) error {
	sessionCookie, err := c.Cookie("webauthn_session")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Session expired"})
	}

	userIDCookie, err := c.Cookie("pending_user_id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Session expired"})
	}

	sessionData, err := base64.StdEncoding.DecodeString(sessionCookie.Value)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid session"})
	}

	var credentialResponse protocol.CredentialCreationResponse
	if err := json.NewDecoder(c.Request().Body).Decode(&credentialResponse); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid credential"})
	}

	parsedResponse, err := credentialResponse.Parse()
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Failed to parse credential"})
	}

	if err := h.passkeys.FinishRegistration(c.Request().Context(), userIDCookie.Value, sessionData, parsedResponse); err != nil {
		c.Logger().Error("Failed to finish registration:", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to register passkey"})
	}

	// Clear WebAuthn cookies
	h.clearWebAuthnCookies(c)

	// Create session
	token, err := h.sessions.Create(c.Request().Context(), userIDCookie.Value, c.Request().UserAgent(), c.RealIP())
	if err != nil {
		c.Logger().Error("Failed to create session:", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create session"})
	}

	h.setSessionCookie(c, token)

	return c.JSON(http.StatusOK, map[string]string{"redirect": "/my/stash"})
}

// PasskeyLogin starts passkey authentication
func (h *AuthHandler) PasskeyLogin(c echo.Context) error {
	phone := c.FormValue("phone")
	if phone == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Phone number is required"})
	}

	phone = auth.NormalizePhoneNumber(phone)

	user, err := h.users.FindByPhone(c.Request().Context(), phone)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "No account found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to find user"})
	}

	options, sessionData, err := h.passkeys.BeginLogin(c.Request().Context(), user.ID)
	if err != nil {
		if errors.Is(err, auth.ErrPasskeyNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "No passkey registered"})
		}
		c.Logger().Error("Failed to begin login:", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to start login"})
	}

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
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Session expired"})
	}

	userIDCookie, err := c.Cookie("pending_user_id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Session expired"})
	}

	sessionData, err := base64.StdEncoding.DecodeString(sessionCookie.Value)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid session"})
	}

	var assertionResponse protocol.CredentialAssertionResponse
	if err := json.NewDecoder(c.Request().Body).Decode(&assertionResponse); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid assertion"})
	}

	parsedResponse, err := assertionResponse.Parse()
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Failed to parse assertion"})
	}

	userID := userIDCookie.Value
	if err := h.passkeys.FinishLogin(c.Request().Context(), userID, sessionData, parsedResponse); err != nil {
		c.Logger().Error("Failed to finish login:", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to verify passkey"})
	}

	h.clearWebAuthnCookies(c)

	token, err := h.sessions.Create(c.Request().Context(), userID, c.Request().UserAgent(), c.RealIP())
	if err != nil {
		c.Logger().Error("Failed to create session:", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create session"})
	}

	h.setSessionCookie(c, token)

	return c.JSON(http.StatusOK, map[string]string{"redirect": "/my/stash"})
}

// SkipPasskey skips passkey setup and creates session
func (h *AuthHandler) SkipPasskey(c echo.Context) error {
	userIDCookie, err := c.Cookie("pending_user_id")
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/login")
	}

	token, err := h.sessions.Create(c.Request().Context(), userIDCookie.Value, c.Request().UserAgent(), c.RealIP())
	if err != nil {
		c.Logger().Error("Failed to create session:", err)
		return c.Redirect(http.StatusSeeOther, "/login")
	}

	h.setSessionCookie(c, token)
	h.clearWebAuthnCookies(c)

	return c.Redirect(http.StatusSeeOther, "/my/stash")
}

// Logout destroys the current session
func (h *AuthHandler) Logout(c echo.Context) error {
	cookie, err := c.Cookie(auth.SessionCookieName)
	if err == nil {
		_ = h.sessions.Delete(c.Request().Context(), cookie.Value)
	}

	c.SetCookie(&http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   c.Request().TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	return c.Redirect(http.StatusSeeOther, "/login")
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
	for _, name := range []string{"webauthn_session", "pending_user_id", "passkey_options"} {
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
