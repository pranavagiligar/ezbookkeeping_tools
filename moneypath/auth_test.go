package main

import (
	"encoding/json"
	"testing"

	"github.com/go-webauthn/webauthn/webauthn"
)

func TestNormalizeLoginEmail(t *testing.T) {
	cases := map[string]string{
		"  User.Name+tag@Example.COM\n": "user.name+tag@example.com",
		"user@example.com":              "user@example.com",
		"":                              "",
	}

	for input, want := range cases {
		if got := normalizeLoginEmail(input); got != want {
			t.Fatalf("normalizeLoginEmail(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestResolveResetTargetEmail(t *testing.T) {
	if got := resolveResetTargetEmail(" User.Name+tag@Example.COM ", "fallback@example.com"); got != "user.name+tag@example.com" {
		t.Fatalf("resolveResetTargetEmail(normalized input) = %q, want %q", got, "user.name+tag@example.com")
	}
	if got := resolveResetTargetEmail("", "fallback@example.com"); got != "fallback@example.com" {
		t.Fatalf("resolveResetTargetEmail(empty input) = %q, want %q", got, "fallback@example.com")
	}
	if got := resolveResetTargetEmail(" second@example.com, first@example.com ", "fallback@example.com"); got != "second@example.com" {
		t.Fatalf("resolveResetTargetEmail(csv input) = %q, want %q", got, "second@example.com")
	}
}

func TestValidatePasskeyInput(t *testing.T) {
	old := passkeyValue
	passkeyValue = "sample-passkey"
	defer func() { passkeyValue = old }()

	if ok := validatePasskeyInput("user@example.com", "sample-passkey"); !ok {
		t.Fatal("validatePasskeyInput should accept a matching email and passkey")
	}
	if ok := validatePasskeyInput("user@example.com", "wrong-passkey"); ok {
		t.Fatal("validatePasskeyInput should reject a mismatched passkey")
	}
}

func TestWebAuthnRegistrationJSONIncludesPublicKeyWrapper(t *testing.T) {
	if err := initWebAuthn(); err != nil {
		t.Fatalf("initWebAuthn() failed: %v", err)
	}

	creation, _, err := webAuthn.BeginRegistration(adminWebUser)
	if err != nil {
		t.Fatalf("BeginRegistration() failed: %v", err)
	}

	payload, err := json.Marshal(creation)
	if err != nil {
		t.Fatalf("json.Marshal(creation) failed: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() failed: %v", err)
	}
	if _, ok := decoded["publicKey"]; !ok {
		t.Fatal("WebAuthn registration JSON must contain a top-level publicKey wrapper for browser clients")
	}
	if _, ok := decoded["publicKey"].(map[string]any)["challenge"]; !ok {
		t.Fatal("publicKey.challenge must be present in the registration payload")
	}
}

func TestCanRegisterAdminPasskeySingleAdminModel(t *testing.T) {
	adminWebUser = &WebAuthnUser{Name: "admin", DisplayName: "Admin"}
	if !canRegisterAdminPasskey() {
		t.Fatal("first admin registration should be allowed")
	}

	adminWebUser.Credentials = append(adminWebUser.Credentials, webauthn.Credential{})
	if canRegisterAdminPasskey() {
		t.Fatal("second admin registration should be blocked once one credential already exists")
	}
}

func TestPersistedAdminStateRoundTrip(t *testing.T) {
	dbPath := t.TempDir() + "/money-path-state.db"
	if err := initStateDB(dbPath); err != nil {
		t.Fatalf("initStateDB() failed: %v", err)
	}
	defer func() { _ = stateDB.Close() }()

	if err := saveStateString("passkey_value", "rotated-passkey"); err != nil {
		t.Fatalf("saveStateString() failed: %v", err)
	}
	if got := loadStateString("passkey_value"); got != "rotated-passkey" {
		t.Fatalf("loadStateString() = %q, want %q", got, "rotated-passkey")
	}

	adminWebUser = &WebAuthnUser{Name: "admin", DisplayName: "Admin"}
	adminWebUser.Credentials = []webauthn.Credential{{
		ID: []byte("credential-123"),
	}}
	if err := persistAdminCredentials(); err != nil {
		t.Fatalf("persistAdminCredentials() failed: %v", err)
	}

	loaded := &WebAuthnUser{Name: "admin", DisplayName: "Admin"}
	if err := loadAdminCredentialsFromStore(loaded); err != nil {
		t.Fatalf("loadAdminCredentialsFromStore() failed: %v", err)
	}
	if len(loaded.Credentials) != 1 || string(loaded.Credentials[0].ID) != "credential-123" {
		t.Fatalf("loaded credentials mismatch: %+v", loaded.Credentials)
	}
}
