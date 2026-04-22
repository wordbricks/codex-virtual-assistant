package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestManagerSessionRequiresCSRFForUnsafeMethods(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t)
	protected := manager.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	loginResponse := httptest.NewRecorder()
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"user_id":"operator","password":"correct horse battery staple"}`))
	manager.HandleLogin(loginResponse, loginRequest)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("HandleLogin() status = %d, want %d; body = %s", loginResponse.Code, http.StatusOK, loginResponse.Body.String())
	}
	var status Status
	if err := json.Unmarshal(loginResponse.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if !status.Authenticated || status.CSRFToken == "" {
		t.Fatalf("login status = %#v, want authenticated with csrf token", status)
	}
	cookies := loginResponse.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("login cookies = %d, want 1", len(cookies))
	}

	getRequest := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	getRequest.AddCookie(cookies[0])
	getResponse := httptest.NewRecorder()
	protected.ServeHTTP(getResponse, getRequest)
	if getResponse.Code != http.StatusNoContent {
		t.Fatalf("GET protected status = %d, want %d", getResponse.Code, http.StatusNoContent)
	}

	postRequest := httptest.NewRequest(http.MethodPost, "/api/v1/projects", nil)
	postRequest.AddCookie(cookies[0])
	postResponse := httptest.NewRecorder()
	protected.ServeHTTP(postResponse, postRequest)
	if postResponse.Code != http.StatusForbidden {
		t.Fatalf("POST without csrf status = %d, want %d", postResponse.Code, http.StatusForbidden)
	}

	csrfRequest := httptest.NewRequest(http.MethodPost, "/api/v1/projects", nil)
	csrfRequest.AddCookie(cookies[0])
	csrfRequest.Header.Set(CSRFHeaderName, status.CSRFToken)
	csrfResponse := httptest.NewRecorder()
	protected.ServeHTTP(csrfResponse, csrfRequest)
	if csrfResponse.Code != http.StatusNoContent {
		t.Fatalf("POST with csrf status = %d, want %d", csrfResponse.Code, http.StatusNoContent)
	}
}

func TestManagerAllowsBasicAuthWithoutCSRF(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t)
	protected := manager.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects", nil)
	request.SetBasicAuth("operator", "correct horse battery staple")
	response := httptest.NewRecorder()
	protected.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("POST with basic auth status = %d, want %d; body = %s", response.Code, http.StatusNoContent, response.Body.String())
	}
}

func TestManagerRejectsMissingCredentials(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t)
	protected := manager.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	response := httptest.NewRecorder()
	protected.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("GET without auth status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()

	hash, err := HashPasswordWithParams("correct horse battery staple", testPasswordParams)
	if err != nil {
		t.Fatalf("HashPasswordWithParams() error = %v", err)
	}
	manager, err := NewManager(Config{
		Enabled:      true,
		UserID:       "operator",
		PasswordHash: hash,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	return manager
}
