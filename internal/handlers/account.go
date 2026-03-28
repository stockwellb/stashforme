package handlers

import (
	"encoding/base64"
	"net/http"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/labstack/echo/v4"

	"stashforme/internal/auth"
	"stashforme/internal/views"
)

// AccountHandler handles account management endpoints
type AccountHandler struct {
	users    *auth.UserStore
	passkeys *auth.PasskeyService
}

// NewAccountHandler creates a new account handler
func NewAccountHandler(users *auth.UserStore, passkeys *auth.PasskeyService) *AccountHandler {
	return &AccountHandler{
		users:    users,
		passkeys: passkeys,
	}
}

// Account renders the account settings page
func (h *AccountHandler) Account(c echo.Context) error {
	user, ok := c.Get("user").(*auth.User)
	if !ok || user == nil {
		return c.Redirect(http.StatusSeeOther, "/login")
	}

	passkeys, err := h.passkeys.ListPasskeys(c.Request().Context(), user.ID)
	if err != nil {
		c.Logger().Error("Failed to list passkeys:", err)
		passkeys = []auth.Passkey{}
	}

	// Prepare registration options for adding new passkey
	options, sessionData, err := h.passkeys.BeginRegistration(c.Request().Context(), user.ID)
	if err != nil {
		c.Logger().Error("Failed to begin registration:", err)
		return Render(c, http.StatusOK, views.Account(user, passkeys, nil))
	}

	// Store session data in cookie
	c.SetCookie(&http.Cookie{
		Name:     "webauthn_session",
		Value:    base64.StdEncoding.EncodeToString(sessionData),
		Path:     "/",
		HttpOnly: true,
		Secure:   c.Request().TLS != nil,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   300,
	})

	userID, _ := options.Response.User.ID.([]byte)
	registerData := &views.PasskeyRegisterData{
		Challenge:       base64.RawURLEncoding.EncodeToString(options.Response.Challenge),
		RPID:            options.Response.RelyingParty.ID,
		RPName:          options.Response.RelyingParty.Name,
		UserID:          base64.RawURLEncoding.EncodeToString(userID),
		UserName:        options.Response.User.Name,
		UserDisplayName: options.Response.User.DisplayName,
	}

	return Render(c, http.StatusOK, views.Account(user, passkeys, registerData))
}

// DeletePasskey removes a passkey
func (h *AccountHandler) DeletePasskey(c echo.Context) error {
	user, ok := c.Get("user").(*auth.User)
	if !ok || user == nil {
		return c.Redirect(http.StatusSeeOther, "/login")
	}

	passkeyID := c.Param("id")
	if passkeyID == "" {
		return c.Redirect(http.StatusSeeOther, "/my/account")
	}

	if err := h.passkeys.DeletePasskey(c.Request().Context(), user.ID, passkeyID); err != nil {
		c.Logger().Error("Failed to delete passkey:", err)
	}

	return c.Redirect(http.StatusSeeOther, "/my/account")
}

// RegisterPasskey handles passkey registration from account page
func (h *AccountHandler) RegisterPasskey(c echo.Context) error {
	user, ok := c.Get("user").(*auth.User)
	if !ok || user == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
	}

	sessionCookie, err := c.Cookie("webauthn_session")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Session expired"})
	}

	sessionData, err := base64.StdEncoding.DecodeString(sessionCookie.Value)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid session"})
	}

	var credentialResponse struct {
		ID       string `json:"id"`
		RawID    string `json:"rawId"`
		Type     string `json:"type"`
		Response struct {
			AttestationObject string `json:"attestationObject"`
			ClientDataJSON    string `json:"clientDataJSON"`
		} `json:"response"`
	}
	if err := c.Bind(&credentialResponse); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	// Decode and parse the credential
	rawID, _ := base64.RawURLEncoding.DecodeString(credentialResponse.RawID)
	attestationObject, _ := base64.RawURLEncoding.DecodeString(credentialResponse.Response.AttestationObject)
	clientDataJSON, _ := base64.RawURLEncoding.DecodeString(credentialResponse.Response.ClientDataJSON)

	parsedResponse, err := parseCredentialCreationResponse(credentialResponse.ID, rawID, attestationObject, clientDataJSON)
	if err != nil {
		c.Logger().Error("Failed to parse credential:", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Failed to parse credential"})
	}

	if err := h.passkeys.FinishRegistration(c.Request().Context(), user.ID, sessionData, parsedResponse); err != nil {
		c.Logger().Error("Failed to finish registration:", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to register passkey"})
	}

	// Clear session cookie
	c.SetCookie(&http.Cookie{
		Name:     "webauthn_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	return c.JSON(http.StatusOK, map[string]string{"redirect": "/my/account"})
}

func parseCredentialCreationResponse(id string, rawID, attestationObject, clientDataJSON []byte) (*protocol.ParsedCredentialCreationData, error) {
	response := protocol.CredentialCreationResponse{
		PublicKeyCredential: protocol.PublicKeyCredential{
			Credential: protocol.Credential{
				ID:   id,
				Type: "public-key",
			},
			RawID: rawID,
		},
		AttestationResponse: protocol.AuthenticatorAttestationResponse{
			AuthenticatorResponse: protocol.AuthenticatorResponse{
				ClientDataJSON: clientDataJSON,
			},
			AttestationObject: attestationObject,
		},
	}
	return response.Parse()
}
