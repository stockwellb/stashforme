package handlers

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"

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
	// Redirect to stash if already logged in
	if cookie, err := c.Cookie(auth.SessionCookieName); err == nil {
		if _, err := h.sessions.Validate(c.Request().Context(), cookie.Value); err == nil {
			return c.Redirect(http.StatusSeeOther, PathMyStash)
		}
	}

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
		return c.Redirect(http.StatusSeeOther, LoginURL(errorMsg))
	}

	return c.Redirect(http.StatusSeeOther, VerifyURL(phone, ""))
}

// VerifyPage renders the verification code entry page
func (h *AuthHandler) VerifyPage(c echo.Context) error {
	phone := c.QueryParam("phone")
	errorMsg := c.QueryParam("error")
	if phone == "" {
		return c.Redirect(http.StatusSeeOther, PathLogin)
	}
	return Render(c, http.StatusOK, views.Verify(phone, errorMsg))
}

// VerifyCode verifies the OTP and redirects appropriately
func (h *AuthHandler) VerifyCode(c echo.Context) error {
	phone := c.FormValue("phone")
	code := c.FormValue("code")

	if phone == "" || code == "" {
		return c.Redirect(http.StatusSeeOther, PathLogin)
	}

	phone = auth.NormalizePhoneNumber(phone)

	if err := h.otp.VerifyCode(c.Request().Context(), phone, code); err != nil {
		errorMsg := "Invalid code"
		if errors.Is(err, auth.ErrOTPExpired) {
			errorMsg = "Code expired"
		} else if errors.Is(err, auth.ErrOTPMaxAttempts) {
			errorMsg = "Too many attempts"
		}
		return c.Redirect(http.StatusSeeOther, VerifyURL(phone, errorMsg))
	}

	user, _, err := h.users.FindOrCreate(c.Request().Context(), phone)
	if err != nil {
		c.Logger().Error("Failed to find/create user:", err)
		return c.Redirect(http.StatusSeeOther, VerifyURL(phone, "Something went wrong"))
	}

	hasPasskey, err := h.passkeys.HasPasskey(c.Request().Context(), user.ID)
	if err != nil {
		c.Logger().Error("Failed to check passkeys:", err)
		return c.Redirect(http.StatusSeeOther, VerifyURL(phone, "Something went wrong"))
	}

	if hasPasskey {
		return h.createSessionAndRedirect(c, user.ID, PathMyStash)
	}

	// No passkey - redirect to passkey setup
	return h.beginPasskeyRegistration(c, user.ID)
}

// PasskeySetupPage renders the passkey setup page
func (h *AuthHandler) PasskeySetupPage(c echo.Context) error {
	optionsCookie, err := c.Cookie(passkeyOptionsCookie)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, PathLogin)
	}

	optionsData, err := base64.StdEncoding.DecodeString(optionsCookie.Value)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, PathLogin)
	}

	var data auth.PasskeyRegisterData
	if err := json.Unmarshal(optionsData, &data); err != nil {
		return c.Redirect(http.StatusSeeOther, PathLogin)
	}

	return Render(c, http.StatusOK, views.PasskeySetup(data))
}

// PasskeyRegister completes passkey registration
func (h *AuthHandler) PasskeyRegister(c echo.Context) error {
	sessionData, err := getWebAuthnSessionData(c)
	if err != nil {
		return jsonError(c, http.StatusBadRequest, "Session expired")
	}

	userID, err := getPendingUserID(c)
	if err != nil {
		return jsonError(c, http.StatusBadRequest, "Session expired")
	}

	parsedResponse, err := h.parseCredentialCreation(c)
	if err != nil {
		c.Logger().Error("Failed to parse credential:", err)
		return jsonError(c, http.StatusBadRequest, "Invalid credential")
	}

	if err := h.passkeys.FinishRegistration(c.Request().Context(), userID, sessionData, parsedResponse); err != nil {
		c.Logger().Error("Failed to finish registration:", err)
		return jsonError(c, http.StatusInternalServerError, "Failed to register passkey")
	}

	clearWebAuthnCookies(c)

	token, err := h.sessions.Create(c.Request().Context(), userID, c.Request().UserAgent(), c.RealIP())
	if err != nil {
		c.Logger().Error("Failed to create session:", err)
		return jsonError(c, http.StatusInternalServerError, "Failed to create session")
	}

	setSessionCookie(c, token)
	return jsonRedirect(c, PathMyStash)
}

// PasskeyLogin starts passkey authentication
func (h *AuthHandler) PasskeyLogin(c echo.Context) error {
	phone := c.FormValue("phone")
	if phone == "" {
		return jsonError(c, http.StatusBadRequest, "Phone number is required")
	}

	phone = auth.NormalizePhoneNumber(phone)

	user, err := h.users.FindByPhone(c.Request().Context(), phone)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			return jsonError(c, http.StatusNotFound, "No account found")
		}
		return jsonError(c, http.StatusInternalServerError, "Failed to find user")
	}

	options, sessionData, err := h.passkeys.BeginLogin(c.Request().Context(), user.ID)
	if err != nil {
		if errors.Is(err, auth.ErrPasskeyNotFound) {
			return jsonError(c, http.StatusNotFound, "No passkey registered")
		}
		c.Logger().Error("Failed to begin login:", err)
		return jsonError(c, http.StatusInternalServerError, "Failed to start login")
	}

	setWebAuthnSessionCookie(c, sessionData)
	setPendingUserCookie(c, user.ID)

	// Return raw WebAuthn options - not wrapped in Response struct
	// because the WebAuthn API expects a specific structure with publicKey
	return c.JSON(http.StatusOK, options)
}

// PasskeyLoginFinish completes passkey authentication
func (h *AuthHandler) PasskeyLoginFinish(c echo.Context) error {
	sessionData, err := getWebAuthnSessionData(c)
	if err != nil {
		return jsonError(c, http.StatusBadRequest, "Session expired")
	}

	userID, err := getPendingUserID(c)
	if err != nil {
		return jsonError(c, http.StatusBadRequest, "Session expired")
	}

	parsedResponse, err := h.parseCredentialAssertion(c)
	if err != nil {
		c.Logger().Error("Failed to parse assertion:", err)
		return jsonError(c, http.StatusBadRequest, "Invalid assertion")
	}

	if err := h.passkeys.FinishLogin(c.Request().Context(), userID, sessionData, parsedResponse); err != nil {
		c.Logger().Error("Failed to finish login:", err)
		return jsonError(c, http.StatusInternalServerError, "Failed to verify passkey")
	}

	clearWebAuthnCookies(c)

	token, err := h.sessions.Create(c.Request().Context(), userID, c.Request().UserAgent(), c.RealIP())
	if err != nil {
		c.Logger().Error("Failed to create session:", err)
		return jsonError(c, http.StatusInternalServerError, "Failed to create session")
	}

	setSessionCookie(c, token)
	return jsonRedirect(c, PathMyStash)
}

// SkipPasskey skips passkey setup and creates session
func (h *AuthHandler) SkipPasskey(c echo.Context) error {
	userID, err := getPendingUserID(c)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, PathLogin)
	}

	token, err := h.sessions.Create(c.Request().Context(), userID, c.Request().UserAgent(), c.RealIP())
	if err != nil {
		c.Logger().Error("Failed to create session:", err)
		return c.Redirect(http.StatusSeeOther, PathLogin)
	}

	setSessionCookie(c, token)
	clearWebAuthnCookies(c)

	return c.Redirect(http.StatusSeeOther, PathMyStash)
}

// Logout destroys the current session
func (h *AuthHandler) Logout(c echo.Context) error {
	cookie, err := c.Cookie(auth.SessionCookieName)
	if err == nil {
		_ = h.sessions.Delete(c.Request().Context(), cookie.Value)
	}

	clearSessionCookie(c)
	return c.Redirect(http.StatusSeeOther, PathLogin)
}

// Helper methods

func (h *AuthHandler) createSessionAndRedirect(c echo.Context, userID, destination string) error {
	token, err := h.sessions.Create(c.Request().Context(), userID, c.Request().UserAgent(), c.RealIP())
	if err != nil {
		c.Logger().Error("Failed to create session:", err)
		return c.Redirect(http.StatusSeeOther, PathLogin)
	}
	setSessionCookie(c, token)
	return c.Redirect(http.StatusSeeOther, destination)
}

func (h *AuthHandler) beginPasskeyRegistration(c echo.Context, userID string) error {
	options, sessionData, err := h.passkeys.BeginRegistration(c.Request().Context(), userID)
	if err != nil {
		c.Logger().Error("Failed to begin registration:", err)
		return h.createSessionAndRedirect(c, userID, PathMyStash)
	}

	setWebAuthnSessionCookie(c, sessionData)
	setPendingUserCookie(c, userID)

	data, err := auth.NewPasskeyRegisterData(options)
	if err != nil {
		c.Logger().Error("Failed to prepare registration data:", err)
		return h.createSessionAndRedirect(c, userID, PathMyStash)
	}

	optionsJSON, _ := json.Marshal(data)
	setPasskeyOptionsCookie(c, optionsJSON)

	return c.Redirect(http.StatusSeeOther, PathPasskeySetup)
}

func (h *AuthHandler) parseCredentialCreation(c echo.Context) (*protocol.ParsedCredentialCreationData, error) {
	var credentialResponse protocol.CredentialCreationResponse
	if err := json.NewDecoder(c.Request().Body).Decode(&credentialResponse); err != nil {
		return nil, err
	}
	return credentialResponse.Parse()
}

func (h *AuthHandler) parseCredentialAssertion(c echo.Context) (*protocol.ParsedCredentialAssertionData, error) {
	var assertionResponse protocol.CredentialAssertionResponse
	if err := json.NewDecoder(c.Request().Body).Decode(&assertionResponse); err != nil {
		return nil, err
	}
	return assertionResponse.Parse()
}
