package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

type WebAuthnUser struct {
	ID          []byte
	Name        string
	DisplayName string
	Credentials []webauthn.Credential
	mu          sync.Mutex
}

func (u *WebAuthnUser) WebAuthnID() []byte {
	return u.ID
}
func (u *WebAuthnUser) WebAuthnName() string {
	return u.Name
}
func (u *WebAuthnUser) WebAuthnDisplayName() string {
	return u.DisplayName
}
func (u *WebAuthnUser) WebAuthnIcon() string { return "" }
func (u *WebAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.Credentials
}

var webAuthn *webauthn.WebAuthn
var adminWebUser *WebAuthnUser
var webauthnSessions = map[string]*webauthn.SessionData{}
var webauthnMu sync.Mutex

func canRegisterAdminPasskey() bool {
	return adminWebUser == nil || len(adminWebUser.Credentials) == 0
}

func persistAdminCredentials() error {
	if adminWebUser == nil || stateDB == nil {
		return nil
	}
	payload, err := json.Marshal(adminWebUser.Credentials)
	if err != nil {
		return err
	}
	return saveStateString("admin_credentials", string(payload))
}

func loadAdminCredentialsFromStore(target *WebAuthnUser) error {
	if target == nil || stateDB == nil {
		return nil
	}
	payload := loadStateString("admin_credentials")
	if strings.TrimSpace(payload) == "" {
		return nil
	}
	var creds []webauthn.Credential
	if err := json.Unmarshal([]byte(payload), &creds); err != nil {
		return err
	}
	target.Credentials = creds
	return nil
}

func initWebAuthn() error {
	rpID := strings.TrimSpace(os.Getenv("WEBAUTHN_RPID"))
	if rpID == "" {
		rpID = "localhost"
	}
	rpOrigin := strings.TrimSpace(os.Getenv("WEBAUTHN_ORIGIN"))
	if rpOrigin == "" {
		rpOrigin = fmt.Sprintf("http://localhost:%d", port)
	}

	var err error
	webAuthn, err = webauthn.New(&webauthn.Config{
		RPDisplayName: "MoneyPath",
		RPID:          rpID,
		RPOrigins:     []string{rpOrigin},
	})
	if err != nil {
		return err
	}

	// single admin user
	adminID := []byte("moneypath-admin")
	adminWebUser = &WebAuthnUser{ID: adminID, Name: "money_pathAdmin", DisplayName: "money_pathAdmin"}
	if err := loadAdminCredentialsFromStore(adminWebUser); err != nil {
		return err
	}
	return nil
}

// WebAuthn registration begin
func handleWebAuthnRegisterBegin(w http.ResponseWriter, r *http.Request) {
	if !canRegisterAdminPasskey() {
		http.Error(w, "admin passkey already registered; this app allows only one admin credential", http.StatusForbidden)
		return
	}
	options, sessionData, err := webAuthn.BeginRegistration(adminWebUser)
	if err != nil {
		http.Error(w, "failed to begin registration", http.StatusInternalServerError)
		return
	}
	webauthnMu.Lock()
	webauthnSessions["register"] = sessionData
	webauthnMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(options)
}

// WebAuthn registration finish
func handleWebAuthnRegisterFinish(w http.ResponseWriter, r *http.Request) {
	webauthnMu.Lock()
	sd := webauthnSessions["register"]
	delete(webauthnSessions, "register")
	webauthnMu.Unlock()
	if sd == nil {
		http.Error(w, "no registration in progress", http.StatusBadRequest)
		return
	}
	cred, err := webAuthn.FinishRegistration(adminWebUser, *sd, r)
	if err != nil {
		http.Error(w, "failed to finish registration: "+err.Error(), http.StatusBadRequest)
		return
	}
	adminWebUser.mu.Lock()
	adminWebUser.Credentials = append(adminWebUser.Credentials, *cred)
	adminWebUser.mu.Unlock()
	if err := persistAdminCredentials(); err != nil {
		http.Error(w, "failed to persist admin credential: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

// WebAuthn authentication begin
func handleWebAuthnAuthBegin(w http.ResponseWriter, r *http.Request) {
	options, sessionData, err := webAuthn.BeginLogin(adminWebUser)
	if err != nil {
		http.Error(w, "failed to begin login", http.StatusInternalServerError)
		return
	}
	webauthnMu.Lock()
	webauthnSessions["login"] = sessionData
	webauthnMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(options)
}

// WebAuthn authentication finish
func handleWebAuthnAuthFinish(w http.ResponseWriter, r *http.Request) {
	webauthnMu.Lock()
	sd := webauthnSessions["login"]
	delete(webauthnSessions, "login")
	webauthnMu.Unlock()
	if sd == nil {
		http.Error(w, "no login in progress", http.StatusBadRequest)
		return
	}
	_, err := webAuthn.FinishLogin(adminWebUser, *sd, r)
	if err != nil {
		http.Error(w, "authentication failed: "+err.Error(), http.StatusUnauthorized)
		return
	}
	// success: create session cookie
	token := generateSecureToken()
	authSessions[token] = adminWebUser.Name
	http.SetCookie(w, &http.Cookie{
		Name:     authSessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int((8 * time.Hour).Seconds()),
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}
