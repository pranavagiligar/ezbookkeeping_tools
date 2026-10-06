package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-gomail/gomail"
	_ "modernc.org/sqlite"

	"github.com/joho/godotenv"
	"github.com/pranavagiligar/ezbookkeeping_tools/moneypath/client"
	"github.com/pranavagiligar/ezbookkeeping_tools/moneypath/models"
	"github.com/pranavagiligar/ezbookkeeping_tools/moneypath/web"
)

var (
	port                  int
	baseURL               string
	apiToken              string
	loginName             string
	password              string
	configFile            string
	defaultSecs           int
	ezClient              *client.EzClient
	mbtilesDB             *sql.DB
	stateDB               *sql.DB
	mbtilesPath           string
	passkeyValue          = "money-path-passkey"
	authSessionCookieName = "moneypath_session"
	authSessions          = map[string]string{}
	pendingResetTokens    = map[string]struct {
		Email     string
		ExpiresAt time.Time
	}{}
)

func init() {
	flag.IntVar(&port, "port", 8081, "Port to run MoneyPath web visualizer on")
	flag.StringVar(&baseURL, "url", "", "ezBookkeeping instance base URL (e.g. https://domain.com)")
	flag.StringVar(&apiToken, "token", "", "ezBookkeeping API Bearer Token")
	flag.StringVar(&loginName, "user", "", "ezBookkeeping username for session auth")
	flag.StringVar(&password, "pass", "", "ezBookkeeping password for session auth")
	flag.StringVar(&configFile, "config", "", "Path to .env configuration file")
	flag.IntVar(&defaultSecs, "duration", 30, "Default fixed phase animation duration in seconds")
}

func loadEnvironment() {
	// Try loading .env in moneypath directory first, then root .env
	envPaths := []string{}
	if configFile != "" {
		envPaths = append(envPaths, configFile)
	}
	envPaths = append(envPaths,
		filepath.Join("moneypath", ".env"),
		".env",
		filepath.Join("..", ".env"),
	)

	loaded := false
	for _, p := range envPaths {
		if _, err := os.Stat(p); err == nil {
			if err := godotenv.Load(p); err == nil {
				fmt.Printf("📄 Loaded environment configuration from: %s\n", p)
				loaded = true
				break
			}
		}
	}
	if !loaded {
		fmt.Println("ℹ️  No .env file found. Running in standalone mode with flags or mock support.")
	}

	if baseURL == "" {
		baseURL = os.Getenv("BASE_URL")
	}
	if apiToken == "" {
		apiToken = os.Getenv("API_TOKEN")
		if apiToken == "" {
			apiToken = os.Getenv("TOKEN")
		}
		if apiToken == "" {
			apiToken = os.Getenv("EBKTOOL_TOKEN")
		}
	}
	if loginName == "" {
		loginName = os.Getenv("LOGIN_NAME")
	}
	if password == "" {
		password = os.Getenv("PASSWORD")
	}
	if pStr := os.Getenv("PORT"); pStr != "" && port == 8081 {
		if pVal, err := strconv.Atoi(pStr); err == nil {
			port = pVal
		}
	}
	if envPasskey := strings.TrimSpace(os.Getenv("PASSKEY")); envPasskey != "" {
		passkeyValue = envPasskey
	}
	if envPasskey := strings.TrimSpace(os.Getenv("APP_PASSKEY")); envPasskey != "" {
		passkeyValue = envPasskey
	}
	// MBTiles path for offline tiles (optional)
	if mb := os.Getenv("MBTILES_PATH"); mb != "" {
		mbtilesPath = mb
	} else {
		// default to ./tiles.mbtiles if present
		if _, err := os.Stat("tiles.mbtiles"); err == nil {
			mbtilesPath = "tiles.mbtiles"
		}
	}
}

func normalizeLoginEmail(raw string) string {
	email := strings.TrimSpace(strings.ToLower(raw))
	if email == "" {
		return ""
	}
	if idx := strings.Index(email, "@"); idx > 0 {
		localPart := email[:idx]
		domain := email[idx+1:]
		localPart = strings.TrimSpace(localPart)
		domain = strings.TrimSpace(domain)
		if localPart == "" || domain == "" {
			return ""
		}
		return localPart + "@" + domain
	}
	return email
}

func parseEmailList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r'
	})
	seen := map[string]bool{}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		email := normalizeLoginEmail(part)
		if email == "" {
			continue
		}
		if !seen[email] {
			seen[email] = true
			out = append(out, email)
		}
	}
	return out
}

func resolveResetTargetEmail(inputEmail, fallback string) string {
	for _, raw := range []string{inputEmail, fallback} {
		for _, email := range parseEmailList(raw) {
			return email
		}
	}
	return ""
}

func validatePasskeyInput(email, passkey string) bool {
	email = normalizeLoginEmail(email)
	if email == "" || passkey == "" {
		return false
	}
	if strings.TrimSpace(passkey) == "" {
		return false
	}
	return strings.TrimSpace(passkey) == passkeyValue
}

func generateSecureToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func resolveStateDBPath() string {
	if dbPath := strings.TrimSpace(os.Getenv("MONEYPATH_DB_PATH")); dbPath != "" {
		return dbPath
	}
	return filepath.Join(".", "moneypath_state.db")
}

func initStateDB(dbPath string) error {
	if dbPath == "" {
		dbPath = resolveStateDBPath()
	}
	if stateDB != nil {
		_ = stateDB.Close()
		stateDB = nil
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS app_state (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		_ = db.Close()
		return err
	}
	stateDB = db
	return nil
}

func saveStateString(key, value string) error {
	if stateDB == nil || strings.TrimSpace(key) == "" {
		return nil
	}
	_, err := stateDB.Exec(`INSERT INTO app_state (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func loadStateString(key string) string {
	if stateDB == nil || strings.TrimSpace(key) == "" {
		return ""
	}
	var value string
	err := stateDB.QueryRow(`SELECT value FROM app_state WHERE key = ?`, key).Scan(&value)
	if err != nil {
		return ""
	}
	return value
}

func isAuthenticatedRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	cookie, err := r.Cookie(authSessionCookieName)
	if err != nil {
		return false
	}
	_, ok := authSessions[cookie.Value]
	return ok
}

func sendResetEmail(recipient, token string) error {
	smtpHost := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	smtpUser := strings.TrimSpace(os.Getenv("SMTP_USER"))
	smtpPass := strings.TrimSpace(os.Getenv("SMTP_PASS"))
	sender := strings.TrimSpace(os.Getenv("SMTP_FROM"))
	if sender == "" {
		sender = smtpUser
	}
	if smtpHost == "" || smtpUser == "" || smtpPass == "" || recipient == "" {
		return fmt.Errorf("SMTP configuration is incomplete")
	}
	portValue := 587
	if p, err := strconv.Atoi(strings.TrimSpace(os.Getenv("SMTP_PORT"))); err == nil && p > 0 {
		portValue = p
	}
	resetURL := fmt.Sprintf("http://localhost:%d/reset?token=%s", port, token)
	m := gomail.NewMessage()
	m.SetHeader("From", sender)
	m.SetHeader("To", recipient)
	m.SetHeader("Subject", "MoneyPath Passkey Reset Request")
	m.SetBody("text/html", fmt.Sprintf(`
		<html>
		<body style="font-family:Arial,sans-serif; line-height:1.5; color:#1f2937;">
			<p>Hello,</p>
			<p>A reset request was received for your MoneyPath access.</p>
			<p><a href="%s" style="display:inline-block;padding:10px 18px;border-radius:8px;background:#2563eb;color:#fff;text-decoration:none;">Reset Passkey</a></p>
			<p>If you did not request this, you can safely ignore this email.</p>
			<p>Reset link: <a href="%s">%s</a></p>
		</body>
		</html>
	`, resetURL, resetURL, resetURL))
	d := gomail.NewDialer(smtpHost, portValue, smtpUser, smtpPass)
	return d.DialAndSend(m)
}

func requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" || r.URL.Path == "/logout" || strings.HasPrefix(r.URL.Path, "/api/auth/") || strings.HasPrefix(r.URL.Path, "/reset") {
			next.ServeHTTP(w, r)
			return
		}
		if !isAuthenticatedRequest(r) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func renderLoginPage() string {
	isPasskeyConfigured := adminWebUser != nil && len(adminWebUser.Credentials) > 0
	buttonClass := "secondary"
	hiddenAttr := ""
	if isPasskeyConfigured {
		buttonClass = "secondary hidden"
		hiddenAttr = "hidden"
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>MoneyPath Login</title>
  <style>
    body { margin:0; font-family:Arial,sans-serif; background:linear-gradient(135deg,#0f172a,#111827 45%%,#1e293b); color:#e2e8f0; min-height:100vh; display:flex; align-items:center; justify-content:center; }
    .card { width:min(460px, 92vw); background:rgba(15,23,42,0.85); border:1px solid rgba(148,163,184,0.25); border-radius:18px; box-shadow:0 24px 80px rgba(0,0,0,0.45); padding:28px; }
    h1 { margin:0 0 8px; font-size:2rem; }
    p { color:#cbd5e1; margin-top:0; }
    .actions { display:grid; gap:12px; margin-top:18px; }
    button { width:100%%; padding:12px 16px; border:none; border-radius:10px; font-weight:700; font-size:1rem; cursor:pointer; }
    .primary { background:linear-gradient(135deg,#3b82f6,#8b5cf6); color:white; }
    .secondary { background:transparent; border:1px solid rgba(148,163,184,0.5); color:#e2e8f0; }
    .muted { font-size:0.82rem; color:#94a3b8; margin-top:12px; }
    #message { margin-top:16px; min-height:24px; font-size:0.95rem; color:#fca5a5; }
    .hidden { display:none !important; }
  </style>
</head>
<body>
  <div class="card">
    <h1>MoneyPath</h1>
    <p>Single-user WebAuthn access for the administrator.</p>
    <div class="actions">
      <button class="primary" id="login-passkey" type="button">Sign in with passkey</button>
      <button class="%s" id="register-passkey" type="button" %s>Register this browser as a passkey</button>
      <button class="secondary" id="reset-button" type="button">Request admin reset email</button>
    </div>
    <div id="message"></div>
    <p class="muted">This app is intended for one owner. The passkey is the secure login, and password reset remains email-only to the configured admin address.</p>
  </div>
  <script>
    const messageBox = document.getElementById('message');
    const webAuthnSupported = !!(window.PublicKeyCredential && window.isSecureContext);
    const registerButton = document.getElementById('register-passkey');
    const isPasskeyConfigured = %t;

    function setMessage(text, isError) {
      messageBox.textContent = text;
      messageBox.style.color = isError ? '#fca5a5' : '#86efac';
    }

    function updateRegistrationState() {
      if (isPasskeyConfigured || registerButton === null) {
        if (registerButton) registerButton.classList.add('hidden');
        return;
      }
      registerButton.classList.remove('hidden');
    }

    function decodeBase64URL(value) {
      const normalized = value.replace(/-/g, '+').replace(/_/g, '/');
      const padding = normalized.length %% 4 === 0 ? '' : '='.repeat(4 - (normalized.length %% 4));
      const binary = atob(normalized + padding);
      const bytes = new Uint8Array(binary.length);
      for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
      return bytes;
    }

    function encodeBase64URL(bytes) {
      let binary = '';
      bytes.forEach((byte) => binary += String.fromCharCode(byte));
      return btoa(binary)
        .replace(/\+/g, '-')
        .replace(/\//g, '_')
        .replace(/=+$/g, '');
    }

    function parsePublicKeyOptions(options) {
      const publicKey = options && options.publicKey ? JSON.parse(JSON.stringify(options.publicKey)) : JSON.parse(JSON.stringify(options));
      if (publicKey.challenge) publicKey.challenge = decodeBase64URL(publicKey.challenge);
      if (publicKey.user && publicKey.user.id) publicKey.user.id = decodeBase64URL(publicKey.user.id);
      if (Array.isArray(publicKey.allowCredentials)) {
        publicKey.allowCredentials = publicKey.allowCredentials.map((entry) => ({
          ...entry,
          id: decodeBase64URL(entry.id)
        }));
      }
      if (Array.isArray(publicKey.excludeCredentials)) {
        publicKey.excludeCredentials = publicKey.excludeCredentials.map((entry) => ({
          ...entry,
          id: decodeBase64URL(entry.id)
        }));
      }
      return publicKey;
    }

    async function webAuthnRequest(url, payload) {
      const response = await fetch(url, {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify(payload || {})
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(data.error || 'WebAuthn request failed');
      return data;
    }

    async function registerPasskey() {
      if (!webAuthnSupported) {
        setMessage('WebAuthn is not available in this browser context. Use HTTPS or localhost.', true);
        return;
      }
      try {
        setMessage('Creating passkey...', false);
        const options = await webAuthnRequest('/api/auth/webauthn/register/begin');
        const publicKey = parsePublicKeyOptions(options);
        const credential = await navigator.credentials.create({ publicKey });
        const response = credential.response;
        const payload = {
          id: credential.id,
          rawId: encodeBase64URL(new Uint8Array(credential.rawId)),
          type: credential.type,
          response: {
            clientDataJSON: encodeBase64URL(new Uint8Array(response.clientDataJSON)),
            attestationObject: encodeBase64URL(new Uint8Array(response.attestationObject))
          }
        };
        await webAuthnRequest('/api/auth/webauthn/register/finish', payload);
        setMessage('Passkey registered successfully. You can now sign in with it.', false);
        if (registerButton) {
          registerButton.classList.add('hidden');
          registerButton.setAttribute('hidden', 'hidden');
        }
      } catch (error) {
        setMessage(error.message || 'Passkey registration failed.', true);
      }
    }

    async function loginWithPasskey() {
      if (!webAuthnSupported) {
        setMessage('WebAuthn is not available in this browser context. Use HTTPS or localhost.', true);
        return;
      }
      try {
        setMessage('Requesting passkey challenge...', false);
        const options = await webAuthnRequest('/api/auth/webauthn/authenticate/begin');
        const publicKey = parsePublicKeyOptions(options);
        const credential = await navigator.credentials.get({ publicKey });
        const response = credential.response;
        const payload = {
          id: credential.id,
          rawId: encodeBase64URL(new Uint8Array(credential.rawId)),
          type: credential.type,
          response: {
            clientDataJSON: encodeBase64URL(new Uint8Array(response.clientDataJSON)),
            authenticatorData: encodeBase64URL(new Uint8Array(response.authenticatorData)),
            signature: encodeBase64URL(new Uint8Array(response.signature)),
            userHandle: response.userHandle ? encodeBase64URL(new Uint8Array(response.userHandle)) : null
          }
        };
        await webAuthnRequest('/api/auth/webauthn/authenticate/finish', payload);
        window.location.href = '/';
      } catch (error) {
        setMessage(error.message || 'Passkey login failed.', true);
      }
    }

    document.getElementById('login-passkey').addEventListener('click', loginWithPasskey);
    document.getElementById('register-passkey').addEventListener('click', registerPasskey);
    document.getElementById('reset-button').addEventListener('click', async () => {
      try {
        const response = await fetch('/api/auth/request-reset', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({})
        });
        const data = await response.json();
        if (!response.ok) throw new Error(data.error || 'Reset request failed');
        setMessage('A reset email has been sent to the configured admin address (if configured).', false);
      } catch (error) {
        setMessage(error.message || 'Reset request failed.', true);
      }
    });

    updateRegistrationState();
  </script>
</body>
</html>`, buttonClass, hiddenAttr, isPasskeyConfigured)
}

func renderResetPage() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>MoneyPath Reset Passkey</title>
  <style>
    body { margin:0; font-family:Arial,sans-serif; background:linear-gradient(135deg,#0f172a,#111827 45%,#1e293b); color:#e2e8f0; min-height:100vh; display:flex; align-items:center; justify-content:center; }
    .card { width:min(420px, 90vw); background:rgba(15,23,42,0.85); border:1px solid rgba(148,163,184,0.25); border-radius:18px; padding:24px; }
    label { display:block; margin-top:14px; color:#94a3b8; font-size:0.75rem; letter-spacing:0.04em; text-transform:uppercase; }
    input { width:100%; box-sizing:border-box; padding:12px 14px; margin-top:8px; border-radius:10px; border:1px solid #475569; background:#0b1220; color:#f8fafc; }
    button { margin-top:20px; width:100%; padding:12px 16px; border:none; border-radius:10px; background:linear-gradient(135deg,#10b981,#34d399); color:white; font-weight:700; cursor:pointer; }
    #message { margin-top:16px; min-height:20px; color:#fca5a5; }
  </style>
</head>
<body>
  <div class="card">
    <h1 style="margin-top:0;">Set a new passkey</h1>
    <form id="reset-form">
      <input type="hidden" id="token" name="token">
      <label for="new-passkey">New Passkey</label>
      <input id="new-passkey" type="password" required>
      <button type="submit">Save new passkey</button>
    </form>
    <div id="message"></div>
  </div>
  <script>
    const params = new URLSearchParams(window.location.search);
    const token = params.get('token');
    document.getElementById('token').value = token || '';
    document.getElementById('reset-form').addEventListener('submit', async (event) => {
      event.preventDefault();
      const passkey = document.getElementById('new-passkey').value.trim();
      const formToken = document.getElementById('token').value.trim();
      if (!formToken || !passkey) {
        document.getElementById('message').textContent = 'A valid reset token and new passkey are required.';
        return;
      }
      try {
        const response = await fetch('/api/auth/reset', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({ token: formToken, newPasskey: passkey })
        });
        const data = await response.json();
        if (!response.ok) throw new Error(data.error || 'Reset failed');
        document.getElementById('message').style.color = '#86efac';
        document.getElementById('message').textContent = 'Passkey updated successfully. Redirecting to login...';
        setTimeout(() => window.location.href = '/login', 1500);
      } catch (error) {
        document.getElementById('message').textContent = error.message || 'Reset failed.';
      }
    });
  </script>
</body>
</html>`
}

func handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if isAuthenticatedRequest(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(renderLoginPage()))
}

func handleResetPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Query().Get("token") == "" {
		http.Error(w, "missing reset token", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(renderResetPage()))
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: authSessionCookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	for token := range authSessions {
		delete(authSessions, token)
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Email   string `json:"email"`
		Passkey string `json:"passkey"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if !validatePasskeyInput(req.Email, req.Passkey) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid email or passkey"})
		return
	}
	email := normalizeLoginEmail(req.Email)
	token := generateSecureToken()
	authSessions[token] = email
	http.SetCookie(w, &http.Cookie{
		Name:     authSessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int((8 * time.Hour).Seconds()),
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "email": email})
}

func handleRequestReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	// Security: always send reset notifications only to the configured admin email(s).
	// When EMAIL_TO is a CSV list, use the first valid recipient to keep the reset
	// flow deterministic and predictable.
	adminEmails := parseEmailList(os.Getenv("EMAIL_TO"))
	if len(adminEmails) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "reset functionality is not configured on this server"})
		return
	}
	target := adminEmails[0]
	token := generateSecureToken()
	pendingResetTokens[token] = struct {
		Email     string
		ExpiresAt time.Time
	}{
		Email:     target,
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}
	if err := sendResetEmail(target, token); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to send reset email: " + err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "recipient": target})
}

func handleResetPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Token      string `json:"token"`
		NewPasskey string `json:"newPasskey"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	record, ok := pendingResetTokens[req.Token]
	if !ok || time.Now().After(record.ExpiresAt) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "reset token is invalid or expired"})
		return
	}
	newPasskey := strings.TrimSpace(req.NewPasskey)
	if newPasskey == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "new passkey is required"})
		return
	}
	passkeyValue = newPasskey
	if err := saveStateString("passkey_value", passkeyValue); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to persist new passkey"})
		return
	}
	delete(pendingResetTokens, req.Token)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "email": record.Email})
}

func main() {
	flag.Parse()
	loadEnvironment()
	if err := initStateDB(""); err != nil {
		log.Fatalf("Failed to initialize MoneyPath state database: %v", err)
	}
	defer stateDB.Close()
	if storedPasskey := loadStateString("passkey_value"); storedPasskey != "" {
		passkeyValue = storedPasskey
	}

	// Initialize WebAuthn subsystem
	if err := initWebAuthn(); err != nil {
		log.Printf("⚠️ WebAuthn initialization failed: %v (passkey endpoints will not work)", err)
	}

	ezClient = client.NewEzClient(baseURL, apiToken, loginName, password)

	// Open MBTiles DB if configured
	if mbtilesPath != "" {
		if _, err := os.Stat(mbtilesPath); err == nil {
			db, err := sql.Open("sqlite", mbtilesPath)
			if err != nil {
				log.Printf("Failed to open MBTiles file %s: %v", mbtilesPath, err)
			} else {
				mbtilesDB = db
				log.Printf("Serving tiles from MBTiles: %s", mbtilesPath)
				defer mbtilesDB.Close()
			}
		}
	}

	mux := http.NewServeMux()

	// Public auth endpoints
	mux.HandleFunc("/login", handleLoginPage)
	mux.HandleFunc("/logout", handleLogout)
	mux.HandleFunc("/reset", handleResetPage)
	mux.HandleFunc("/api/auth/login", handleAuthLogin)
	mux.HandleFunc("/api/auth/request-reset", handleRequestReset)
	mux.HandleFunc("/api/auth/reset", handleResetPassword)
	// WebAuthn endpoints (passkey)
	mux.HandleFunc("/api/auth/webauthn/register/begin", handleWebAuthnRegisterBegin)
	mux.HandleFunc("/api/auth/webauthn/register/finish", handleWebAuthnRegisterFinish)
	mux.HandleFunc("/api/auth/webauthn/authenticate/begin", handleWebAuthnAuthBegin)
	mux.HandleFunc("/api/auth/webauthn/authenticate/finish", handleWebAuthnAuthFinish)

	// API Endpoints
	mux.Handle("/api/transactions", requireAuth(http.HandlerFunc(handleTransactions)))
	mux.Handle("/api/mock", requireAuth(http.HandlerFunc(handleMock)))
	mux.Handle("/export/transaction/csv", requireAuth(http.HandlerFunc(handleExportTransactionCSV)))
	mux.Handle("/export/transaction/geojson", requireAuth(http.HandlerFunc(handleExportTransactionGeoJSON)))
	mux.HandleFunc("/api/health", handleHealth)

	// Expose lightweight config endpoint for frontend (e.g. CARTO API key)
	mux.Handle("/config", requireAuth(http.HandlerFunc(handleConfig)))

	// Embedded Static Assets
	fileSystem, err := web.GetFileSystem()
	if err != nil {
		log.Fatalf("Failed to initialize embedded web filesystem: %v", err)
	}
	mux.Handle("/", requireAuth(http.FileServer(fileSystem)))

	// Local tile server (serve files from ./tiles/{z}/{x}/{y}.png)
	mux.Handle("/tiles/", requireAuth(http.HandlerFunc(handleLocalTiles)))

	addr := fmt.Sprintf(":%d", port)
	banner := `
  __  __                         _____      _   _     
 |  \/  |                       |  __ \    | | | |    
 | \  / | ___  _ __   ___ _   _ | |__) |_ _| |_| |__  
 | |\/| |/ _ \| '_ \ / _ \ | | ||  ___/ _' | __| '_ \ 
 | |  | | (_) | | | |  __/ |_| || |  | (_| | |_| | | |
 |_|  |_|\___/|_| |_|\___|\__, ||_|   \__,_|\__|_| |_|
                           __/ |                      
                          |___/                       
  🗺️  ezBookkeeping Geotag Route Animator
`
	fmt.Print(banner)
	fmt.Printf("🚀 MoneyPath server is running at: http://localhost:%d\n", port)
	if baseURL != "" {
		fmt.Printf("🔗 Connected to ezBookkeeping instance: %s\n", baseURL)
	} else {
		fmt.Printf("⚡ ezBookkeeping API not configured. Please configure EZBOOKKEEPING_BASE_URL and EZBOOKKEEPING_TOKEN in .env\n")
	}
	fmt.Printf("⏱️  Default Animation Phase: %d seconds\n\n", defaultSecs)

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}

func handleTransactions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	q := r.URL.Query()
	var minTime, maxTime int64
	if minStr := q.Get("min_time"); minStr != "" {
		minTime, _ = strconv.ParseInt(minStr, 10, 64)
	}
	if maxStr := q.Get("max_time"); maxStr != "" {
		maxTime, _ = strconv.ParseInt(maxStr, 10, 64)
	}

	var categoryIDs []string
	if catStr := q.Get("category_ids"); catStr != "" {
		categoryIDs = strings.Split(catStr, ",")
	}
	var accountIDs []string
	if accStr := q.Get("account_ids"); accStr != "" {
		accountIDs = strings.Split(accStr, ",")
	}

	var limit int
	if limStr := q.Get("limit"); limStr != "" {
		limit, _ = strconv.Atoi(limStr)
	}

	swapCoords := q.Get("swap_coords") == "true" || q.Get("swap_coords") == "1"

	params := models.FilterParams{
		MinTime:         minTime,
		MaxTime:         maxTime,
		CategoryIDs:     categoryIDs,
		AccountIDs:      accountIDs,
		Limit:           limit,
		SwapCoordinates: swapCoords,
	}

	// If ezBookkeeping credentials are not provided, return mock data with notice
	if ezClient.BaseURL == "" || (ezClient.APIToken == "" && (ezClient.LoginName == "" || ezClient.Password == "")) {
		mockResp := client.GenerateMockData(3000)
		mockResp.ErrorMessage = "ezBookkeeping credentials not set in .env. Displaying 3,000 synthetic GPS points."
		_ = json.NewEncoder(w).Encode(mockResp)
		return
	}

	resp, err := ezClient.FetchTransactions(params)
	if err != nil {
		w.WriteHeader(http.StatusOK) // Return JSON error for frontend to handle gracefully
		_ = json.NewEncoder(w).Encode(models.MoneyPathResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		})
		return
	}

	_ = json.NewEncoder(w).Encode(resp)
}

func handleMock(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	count := 3000
	if cStr := r.URL.Query().Get("count"); cStr != "" {
		if cVal, err := strconv.Atoi(cStr); err == nil && cVal > 0 {
			count = cVal
		}
	}

	resp := client.GenerateMockData(count)
	_ = json.NewEncoder(w).Encode(resp)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "ok",
		"version":     "1.0.0",
		"has_api_url": ezClient.BaseURL != "",
	})
}

// handleConfig returns a small JSON payload with public configuration values
// consumed by the MoneyPath frontend (e.g. optional CARTO API key).
func handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	cfg := map[string]interface{}{
		"carto_api_key":              os.Getenv("CARTO_API_KEY"),
		"default_animation_duration": defaultSecs,
	}
	_ = json.NewEncoder(w).Encode(cfg)
}

// export helpers
func writeCSVDownload(w http.ResponseWriter, fileName, csv string) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileName))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(csv))
}

func writeGeoJSONDownload(w http.ResponseWriter, fileName, geo string) {
	w.Header().Set("Content-Type", "application/geo+json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileName))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(geo))
}

// handleExportTransactionCSV serves a single transaction as CSV
func handleExportTransactionCSV(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id := q.Get("id")
	if id == "" {
		http.Error(w, "missing id parameter", http.StatusBadRequest)
		return
	}

	// Reuse the FetchTransactions path to obtain points (apply optional filters)
	params := models.FilterParams{}
	if minStr := q.Get("min_time"); minStr != "" {
		if v, err := strconv.ParseInt(minStr, 10, 64); err == nil {
			params.MinTime = v
		}
	}
	if maxStr := q.Get("max_time"); maxStr != "" {
		if v, err := strconv.ParseInt(maxStr, 10, 64); err == nil {
			params.MaxTime = v
		}
	}
	if q.Get("swap_coords") == "true" {
		params.SwapCoordinates = true
	}

	var resp *models.MoneyPathResponse
	if ezClient.BaseURL == "" {
		resp = client.GenerateMockData(3000)
	} else {
		rresp, err := ezClient.FetchTransactions(params)
		if err != nil {
			http.Error(w, "failed to fetch transactions", http.StatusInternalServerError)
			return
		}
		resp = rresp
	}

	// find point by id
	var found *models.MoneyPathPoint
	for i := range resp.Points {
		if resp.Points[i].ID == id {
			found = &resp.Points[i]
			break
		}
	}
	if found == nil {
		http.Error(w, "transaction not found", http.StatusNotFound)
		return
	}

	// Build CSV
	csv := fmt.Sprintf("ID,Timestamp,FormattedTime,Latitude,Longitude,Category,Account,SourceAccount,DestinationAccount,Amount,Currency,Comment\n%v,%v,\"%v\",%v,%v,\"%v\",\"%v\",\"%v\",\"%v\",%v,%v,\"%v\"",
		found.ID, found.Timestamp, found.FormattedTime, found.Latitude, found.Longitude,
		found.CategoryName, found.AccountName, found.SourceAccountName, found.DestinationAccountName, found.Amount, found.Currency, strings.ReplaceAll(found.Comment, "\"", "\"\""))

	writeCSVDownload(w, fmt.Sprintf("transaction_%s.csv", id), csv)
}

// handleExportTransactionGeoJSON serves a single transaction as GeoJSON
func handleExportTransactionGeoJSON(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id := q.Get("id")
	if id == "" {
		http.Error(w, "missing id parameter", http.StatusBadRequest)
		return
	}

	params := models.FilterParams{}
	if minStr := q.Get("min_time"); minStr != "" {
		if v, err := strconv.ParseInt(minStr, 10, 64); err == nil {
			params.MinTime = v
		}
	}
	if maxStr := q.Get("max_time"); maxStr != "" {
		if v, err := strconv.ParseInt(maxStr, 10, 64); err == nil {
			params.MaxTime = v
		}
	}
	if q.Get("swap_coords") == "true" {
		params.SwapCoordinates = true
	}

	var resp *models.MoneyPathResponse
	if ezClient.BaseURL == "" {
		resp = client.GenerateMockData(3000)
	} else {
		rresp, err := ezClient.FetchTransactions(params)
		if err != nil {
			http.Error(w, "failed to fetch transactions", http.StatusInternalServerError)
			return
		}
		resp = rresp
	}

	var found *models.MoneyPathPoint
	for i := range resp.Points {
		if resp.Points[i].ID == id {
			found = &resp.Points[i]
			break
		}
	}
	if found == nil {
		http.Error(w, "transaction not found", http.StatusNotFound)
		return
	}

	feature := map[string]interface{}{
		"type": "Feature",
		"properties": map[string]interface{}{
			"id":                 found.ID,
			"timestamp":          found.Timestamp,
			"formattedTime":      found.FormattedTime,
			"category":           found.CategoryName,
			"account":            found.AccountName,
			"sourceAccount":      found.SourceAccountName,
			"destinationAccount": found.DestinationAccountName,
			"amount":             found.Amount,
			"comment":            found.Comment,
		},
		"geometry": map[string]interface{}{
			"type":        "Point",
			"coordinates": []float64{found.Longitude, found.Latitude},
		},
	}
	fc := map[string]interface{}{
		"type":     "FeatureCollection",
		"features": []interface{}{feature},
	}
	geoBytes, _ := json.MarshalIndent(fc, "", "  ")
	writeGeoJSONDownload(w, fmt.Sprintf("transaction_%s.geojson", id), string(geoBytes))
}

// handleLocalTiles serves tiles from a local directory structure (tiles/{z}/{x}/{y}.png)
func handleLocalTiles(w http.ResponseWriter, r *http.Request) {
	// Expected path: /tiles/{z}/{x}/{y}.png
	// Trim the prefix
	p := strings.TrimPrefix(r.URL.Path, "/tiles/")
	// Prevent path traversal
	if strings.Contains(p, "..") {
		http.Error(w, "invalid tile path", http.StatusBadRequest)
		return
	}

	// If MBTiles DB is available, try to serve from it first
	if mbtilesDB != nil {
		parts := strings.Split(p, "/")
		if len(parts) >= 3 {
			z, errZ := strconv.Atoi(parts[0])
			x, errX := strconv.Atoi(parts[1])
			// strip extension from y (e.g., y.png)
			yPart := parts[2]
			yStr := strings.TrimSuffix(yPart, filepath.Ext(yPart))
			y, errY := strconv.Atoi(yStr)
			if errZ == nil && errX == nil && errY == nil {
				// convert to TMS row if necessary (MBTiles uses TMS)
				tmsY := (1 << uint(z)) - 1 - y
				var tileData []byte
				query := "SELECT tile_data FROM tiles WHERE zoom_level=? AND tile_column=? AND tile_row=? LIMIT 1"
				row := mbtilesDB.QueryRow(query, z, x, tmsY)
				if err := row.Scan(&tileData); err == nil {
					w.Header().Set("Content-Type", "image/png")
					w.Header().Set("Cache-Control", "public, max-age=86400")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(tileData)
					return
				}
			}
		}
	}

	// Map to local file under ./tiles/ as fallback
	localPath := filepath.Join("tiles", filepath.FromSlash(p))
	if _, err := os.Stat(localPath); err != nil {
		http.NotFound(w, r)
		return
	}

	// Serve file with proper content type
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, localPath)
}
