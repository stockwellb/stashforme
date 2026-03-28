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
	user := RequireUser(c)
	if user == nil {
		return c.Redirect(http.StatusSeeOther, PathLogin)
	}

	passkeys, err := h.passkeys.ListPasskeys(c.Request().Context(), user.ID)
	if err != nil {
		c.Logger().Error("Failed to list passkeys:", err)
		passkeys = []auth.Passkey{}
	}

	registerData, err := h.preparePasskeyRegistration(c, user.ID)
	if err != nil {
		c.Logger().Error("Failed to prepare passkey registration:", err)
		return Render(c, http.StatusOK, views.Account(user, passkeys, nil))
	}

	return Render(c, http.StatusOK, views.Account(user, passkeys, registerData))
}

// DeletePasskey removes a passkey (DELETE method)
func (h *AccountHandler) DeletePasskey(c echo.Context) error {
	user := RequireUser(c)
	if user == nil {
		return c.Redirect(http.StatusSeeOther, PathLogin)
	}

	passkeyID := c.Param("id")
	if passkeyID == "" {
		return c.Redirect(http.StatusSeeOther, PathMyAccount)
	}

	if err := h.passkeys.DeletePasskey(c.Request().Context(), user.ID, passkeyID); err != nil {
		c.Logger().Error("Failed to delete passkey:", err)
	}

	return c.Redirect(http.StatusSeeOther, PathMyAccount)
}

// RegisterPasskey handles passkey registration from account page
func (h *AccountHandler) RegisterPasskey(c echo.Context) error {
	user := RequireUser(c)
	if user == nil {
		return jsonError(c, http.StatusUnauthorized, "Not authenticated")
	}

	sessionData, err := getWebAuthnSessionData(c)
	if err != nil {
		return jsonError(c, http.StatusBadRequest, "Session expired")
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
		return jsonError(c, http.StatusBadRequest, "Invalid request")
	}

	parsedResponse, err := parseCredentialCreationFromJSON(credentialResponse)
	if err != nil {
		c.Logger().Error("Failed to parse credential:", err)
		return jsonError(c, http.StatusBadRequest, "Failed to parse credential")
	}

	if err := h.passkeys.FinishRegistration(c.Request().Context(), user.ID, sessionData, parsedResponse); err != nil {
		c.Logger().Error("Failed to finish registration:", err)
		return jsonError(c, http.StatusInternalServerError, "Failed to register passkey")
	}

	clearWebAuthnCookies(c)
	return jsonRedirect(c, PathMyAccount)
}

// Helper methods

func (h *AccountHandler) preparePasskeyRegistration(c echo.Context, userID string) (*auth.PasskeyRegisterData, error) {
	options, sessionData, err := h.passkeys.BeginRegistration(c.Request().Context(), userID)
	if err != nil {
		return nil, err
	}

	setWebAuthnSessionCookie(c, sessionData)

	userIDBinary, _ := options.Response.User.ID.([]byte)
	data := &auth.PasskeyRegisterData{
		Challenge:       base64.RawURLEncoding.EncodeToString(options.Response.Challenge),
		RPID:            options.Response.RelyingParty.ID,
		RPName:          options.Response.RelyingParty.Name,
		UserID:          base64.RawURLEncoding.EncodeToString(userIDBinary),
		UserName:        options.Response.User.Name,
		UserDisplayName: options.Response.User.DisplayName,
	}

	return data, nil
}

func parseCredentialCreationFromJSON(data struct {
	ID       string `json:"id"`
	RawID    string `json:"rawId"`
	Type     string `json:"type"`
	Response struct {
		AttestationObject string `json:"attestationObject"`
		ClientDataJSON    string `json:"clientDataJSON"`
	} `json:"response"`
}) (*protocol.ParsedCredentialCreationData, error) {
	rawID, _ := base64.RawURLEncoding.DecodeString(data.RawID)
	attestationObject, _ := base64.RawURLEncoding.DecodeString(data.Response.AttestationObject)
	clientDataJSON, _ := base64.RawURLEncoding.DecodeString(data.Response.ClientDataJSON)

	response := protocol.CredentialCreationResponse{
		PublicKeyCredential: protocol.PublicKeyCredential{
			Credential: protocol.Credential{
				ID:   data.ID,
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
