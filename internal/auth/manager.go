package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	SessionCookieName = "cva_session"
	CSRFHeaderName    = "X-CVA-CSRF"
)

type Config struct {
	Enabled      bool
	UserID       string
	PasswordHash string
	Password     string
}

type Status struct {
	Enabled       bool   `json:"enabled"`
	Authenticated bool   `json:"authenticated"`
	UserID        string `json:"user_id,omitempty"`
	CSRFToken     string `json:"csrf_token,omitempty"`
}

type Manager struct {
	enabled      bool
	userID       string
	passwordHash string
	now          func() time.Time
	sessionTTL   time.Duration
	idleTTL      time.Duration

	mu       sync.Mutex
	sessions map[string]*session
	failures map[string]*failureState
}

type session struct {
	UserID    string
	CSRFToken string
	CreatedAt time.Time
	LastSeen  time.Time
	ExpiresAt time.Time
}

type failureState struct {
	Count       int
	FirstSeen   time.Time
	LastSeen    time.Time
	LockedUntil time.Time
}

type contextKey string

const userIDContextKey contextKey = "cva-auth-user-id"

func NewManager(cfg Config) (*Manager, error) {
	manager := &Manager{
		enabled:    cfg.Enabled,
		now:        time.Now,
		sessionTTL: 12 * time.Hour,
		idleTTL:    2 * time.Hour,
		sessions:   make(map[string]*session),
		failures:   make(map[string]*failureState),
	}
	if !cfg.Enabled {
		return manager, nil
	}

	userID := strings.TrimSpace(cfg.UserID)
	if userID == "" {
		userID = "admin"
	}
	if err := ValidateUserID(userID); err != nil {
		return nil, fmt.Errorf("auth user id: %w", err)
	}
	manager.userID = userID

	passwordHash := strings.TrimSpace(cfg.PasswordHash)
	if passwordHash == "" {
		if cfg.Password == "" {
			return nil, errors.New("auth password or password hash is required")
		}
		generated, err := HashPassword(cfg.Password)
		if err != nil {
			return nil, fmt.Errorf("auth password: %w", err)
		}
		passwordHash = generated
	}
	if err := ValidatePasswordHash(passwordHash); err != nil {
		return nil, fmt.Errorf("auth password hash: %w", err)
	}
	manager.passwordHash = passwordHash
	return manager, nil
}

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func UserIDFromContext(ctx context.Context) (string, bool) {
	value, ok := ctx.Value(userIDContextKey).(string)
	return value, ok && value != ""
}

func (m *Manager) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.enabled {
			next.ServeHTTP(w, r)
			return
		}
		if userID, ok := m.basicUserID(r); ok {
			next.ServeHTTP(w, withUserID(r, userID))
			return
		}
		tokenHash, sess, ok := m.sessionFromRequest(r)
		if !ok {
			writeAuthError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		if !safeMethod(r.Method) && !m.validCSRF(r, sess) {
			writeAuthError(w, http.StatusForbidden, "invalid csrf token")
			return
		}
		m.touchSession(tokenHash)
		next.ServeHTTP(w, withUserID(r, sess.UserID))
	})
}

func (m *Manager) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status := Status{Enabled: m.enabled, Authenticated: !m.enabled}
	if !m.enabled {
		writeAuthJSON(w, http.StatusOK, status)
		return
	}
	if userID, ok := m.basicUserID(r); ok {
		status.Authenticated = true
		status.UserID = userID
		writeAuthJSON(w, http.StatusOK, status)
		return
	}
	_, sess, ok := m.sessionFromRequest(r)
	if ok {
		status.Authenticated = true
		status.UserID = sess.UserID
		status.CSRFToken = sess.CSRFToken
	}
	writeAuthJSON(w, http.StatusOK, status)
}

func (m *Manager) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !m.enabled {
		writeAuthJSON(w, http.StatusOK, Status{Enabled: false, Authenticated: true})
		return
	}

	var request struct {
		UserID   string `json:"user_id"`
		Password string `json:"password"`
	}
	if err := decodeAuthJSON(w, r, &request); err != nil {
		writeAuthError(w, http.StatusBadRequest, err.Error())
		return
	}

	ok, retryAfter, err := m.authenticateCredentials(remoteIP(r), request.UserID, request.Password)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "authentication failed")
		return
	}
	if !ok {
		if retryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
			writeAuthError(w, http.StatusTooManyRequests, "invalid credentials")
			return
		}
		writeAuthError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	rawToken, sess, err := m.createSession()
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "create session failed")
		return
	}
	setSessionCookie(w, r, rawToken, int(m.sessionTTL.Seconds()))
	writeAuthJSON(w, http.StatusOK, Status{
		Enabled:       true,
		Authenticated: true,
		UserID:        sess.UserID,
		CSRFToken:     sess.CSRFToken,
	})
}

func (m *Manager) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if m.enabled {
		tokenHash, sess, ok := m.sessionFromRequest(r)
		if ok {
			if !m.validCSRF(r, sess) {
				writeAuthError(w, http.StatusForbidden, "invalid csrf token")
				return
			}
			m.mu.Lock()
			delete(m.sessions, tokenHash)
			m.mu.Unlock()
		}
	}
	clearSessionCookie(w, r)
	writeAuthJSON(w, http.StatusOK, Status{Enabled: m.enabled, Authenticated: !m.enabled})
}

func (m *Manager) basicUserID(r *http.Request) (string, bool) {
	userID, password, ok := r.BasicAuth()
	if !ok {
		return "", false
	}
	ok, _, err := m.authenticateCredentials(remoteIP(r), userID, password)
	if err != nil || !ok {
		return "", false
	}
	return m.userID, true
}

func (m *Manager) authenticateCredentials(remoteAddr, userID, password string) (bool, time.Duration, error) {
	now := m.now()
	key := remoteAddr + "\x00" + strings.TrimSpace(userID)

	m.mu.Lock()
	failure := m.failures[key]
	if failure != nil && now.Before(failure.LockedUntil) {
		retryAfter := failure.LockedUntil.Sub(now).Round(time.Second)
		m.mu.Unlock()
		return false, retryAfter, nil
	}
	m.mu.Unlock()

	passwordOK, err := VerifyPassword(password, m.passwordHash)
	if err != nil {
		return false, 0, err
	}
	userOK := subtle.ConstantTimeCompare([]byte(userID), []byte(m.userID)) == 1
	if userOK && passwordOK {
		m.mu.Lock()
		delete(m.failures, key)
		m.mu.Unlock()
		return true, 0, nil
	}

	m.recordFailure(key, now)
	return false, 0, nil
}

func (m *Manager) recordFailure(key string, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	failure := m.failures[key]
	if failure == nil || now.Sub(failure.FirstSeen) > 15*time.Minute {
		failure = &failureState{FirstSeen: now}
		m.failures[key] = failure
	}
	failure.Count++
	failure.LastSeen = now
	if failure.Count >= 5 {
		backoff := time.Duration(1<<min(failure.Count-5, 8)) * time.Second
		if backoff > 15*time.Minute {
			backoff = 15 * time.Minute
		}
		failure.LockedUntil = now.Add(backoff)
	}
}

func (m *Manager) createSession() (string, *session, error) {
	rawToken, err := randomToken(32)
	if err != nil {
		return "", nil, err
	}
	csrfToken, err := randomToken(32)
	if err != nil {
		return "", nil, err
	}
	now := m.now()
	sess := &session{
		UserID:    m.userID,
		CSRFToken: csrfToken,
		CreatedAt: now,
		LastSeen:  now,
		ExpiresAt: now.Add(m.sessionTTL),
	}

	m.mu.Lock()
	m.sessions[tokenDigest(rawToken)] = sess
	m.mu.Unlock()
	return rawToken, sess, nil
}

func (m *Manager) sessionFromRequest(r *http.Request) (string, *session, bool) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		return "", nil, false
	}
	tokenHash := tokenDigest(cookie.Value)
	now := m.now()

	m.mu.Lock()
	defer m.mu.Unlock()
	sess := m.sessions[tokenHash]
	if sess == nil {
		return "", nil, false
	}
	if now.After(sess.ExpiresAt) || now.Sub(sess.LastSeen) > m.idleTTL {
		delete(m.sessions, tokenHash)
		return "", nil, false
	}
	copied := *sess
	return tokenHash, &copied, true
}

func (m *Manager) touchSession(tokenHash string) {
	now := m.now()
	m.mu.Lock()
	if sess := m.sessions[tokenHash]; sess != nil {
		sess.LastSeen = now
	}
	m.mu.Unlock()
}

func (m *Manager) validCSRF(r *http.Request, sess *session) bool {
	token := r.Header.Get(CSRFHeaderName)
	return token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(sess.CSRFToken)) == 1
}

func decodeAuthJSON(w http.ResponseWriter, r *http.Request, out any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}
	var extra struct{}
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("decode json: multiple values")
	}
	return nil
}

func writeAuthJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeAuthError(w http.ResponseWriter, status int, message string) {
	if status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Basic realm="CVA", charset="UTF-8"`)
	}
	writeAuthJSON(w, status, map[string]string{"error": message})
}

func safeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func withUserID(r *http.Request, userID string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userIDContextKey, userID))
}

func randomToken(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func tokenDigest(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   secureRequest(r),
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   secureRequest(r),
	})
}

func secureRequest(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	if r.RemoteAddr != "" {
		return r.RemoteAddr
	}
	return "unknown"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
