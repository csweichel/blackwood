package granola

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	// Granola's OAuth endpoints from /.well-known/oauth-authorization-server.
	granolaAuthorizationURL = "https://mcp-auth.granola.ai/oauth2/authorize"
	granolaTokenURL         = "https://mcp-auth.granola.ai/oauth2/token"
	granolaRegistrationURL  = "https://mcp-auth.granola.ai/oauth2/register"

	// Refresh the token when it has less than this much time remaining.
	tokenRefreshThreshold = 5 * time.Minute
)

// OAuthToken holds the result of an OAuth token exchange.
type OAuthToken struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// PersistedToken is the on-disk format for the token file. It includes the
// client registration info needed for refresh, and the computed expiry time.
type PersistedToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"client_secret,omitempty"`
	TokenURL     string    `json:"token_url,omitempty"`
}

// SaveToken writes a PersistedToken to disk as JSON with mode 0600.
func SaveToken(path string, pt *PersistedToken) error {
	data, err := json.MarshalIndent(pt, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal token: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// LoadToken reads a PersistedToken from disk. If the file contains a plain
// string (legacy format), it returns a token with just the access token set.
func LoadToken(path string) (*PersistedToken, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	data = []byte(strings.TrimSpace(string(data)))

	// Try JSON first.
	var pt PersistedToken
	if err := json.Unmarshal(data, &pt); err == nil && pt.AccessToken != "" {
		if pt.TokenURL == "" {
			pt.TokenURL = granolaTokenURL
		}
		return &pt, nil
	}

	// Legacy: plain access token string.
	return &PersistedToken{
		AccessToken: string(data),
		TokenURL:    granolaTokenURL,
	}, nil
}

// TokenSource provides access tokens with automatic refresh.
type TokenSource struct {
	mu       sync.Mutex
	token    *PersistedToken
	filePath string // if set, updated tokens are persisted here
}

// NewTokenSource creates a TokenSource from a PersistedToken.
// If filePath is non-empty, refreshed tokens are written back to disk.
func NewTokenSource(token *PersistedToken, filePath string) *TokenSource {
	return &TokenSource{token: token, filePath: filePath}
}

// AccessToken returns a valid access token, refreshing if necessary.
func (ts *TokenSource) AccessToken(ctx context.Context) (string, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.needsRefresh() {
		if err := ts.refresh(ctx); err != nil {
			// If refresh fails but we still have an access token, use it
			// (it might still be valid despite our expiry estimate).
			if ts.token.AccessToken != "" {
				slog.Warn("token refresh failed, using existing token", "error", err)
				return ts.token.AccessToken, nil
			}
			return "", fmt.Errorf("token refresh: %w", err)
		}
	}

	return ts.token.AccessToken, nil
}

func (ts *TokenSource) needsRefresh() bool {
	if ts.token.RefreshToken == "" {
		return false // can't refresh without a refresh token
	}
	if ts.token.ExpiresAt.IsZero() {
		return false // no expiry info, assume valid
	}
	return time.Until(ts.token.ExpiresAt) < tokenRefreshThreshold
}

func (ts *TokenSource) refresh(ctx context.Context) error {
	if ts.token.RefreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}

	tokenURL := ts.token.TokenURL
	if tokenURL == "" {
		tokenURL = granolaTokenURL
	}

	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {ts.token.RefreshToken},
	}
	if ts.token.ClientID != "" {
		data.Set("client_id", ts.token.ClientID)
	}
	if ts.token.ClientSecret != "" {
		data.Set("client_secret", ts.token.ClientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("refresh request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("refresh failed: HTTP %d: %s", resp.StatusCode, string(b))
	}

	var tok OAuthToken
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return fmt.Errorf("parse refresh response: %w", err)
	}

	// Update the persisted token.
	ts.token.AccessToken = tok.AccessToken
	if tok.RefreshToken != "" {
		ts.token.RefreshToken = tok.RefreshToken
	}
	if tok.ExpiresIn > 0 {
		ts.token.ExpiresAt = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	}

	slog.Info("granola OAuth token refreshed", "expires_at", ts.token.ExpiresAt)

	// Persist to disk if configured.
	if ts.filePath != "" {
		if err := SaveToken(ts.filePath, ts.token); err != nil {
			slog.Warn("failed to persist refreshed token", "error", err)
		}
	}

	return nil
}

// dynamicClientRegistration registers a new OAuth client with Granola.
type registrationResponse struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
}

// RegisterClient performs OAuth 2.0 Dynamic Client Registration.
func RegisterClient(ctx context.Context, redirectURI string) (*registrationResponse, error) {
	body := fmt.Sprintf(`{
		"client_name": "Blackwood",
		"redirect_uris": [%q],
		"grant_types": ["authorization_code"],
		"response_types": ["code"],
		"token_endpoint_auth_method": "none"
	}`, redirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, granolaRegistrationURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("register client: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("register client: HTTP %d: %s", resp.StatusCode, string(b))
	}

	var reg registrationResponse
	if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
		return nil, fmt.Errorf("parse registration: %w", err)
	}
	return &reg, nil
}

// LoginResult holds the authorization URL and a function to exchange the
// authorization code for a token.
type LoginResult struct {
	// AuthURL is the URL the user must visit to authorize.
	AuthURL string
	// ClientID from Dynamic Client Registration (needed for token refresh).
	ClientID string
	// ClientSecret from Dynamic Client Registration (may be empty).
	ClientSecret string
	// RedirectURI used for the authorization request.
	RedirectURI string
	// Exchange trades an authorization code for a token. The code is
	// extracted from the redirect URL the user sees after authorizing.
	Exchange func(ctx context.Context, code string) (*OAuthToken, error)
}

// generateCodeVerifier creates a random PKCE code verifier.
func generateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// codeChallenge computes the S256 PKCE code challenge from a verifier.
func codeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// oobRedirectURI is a well-known redirect URI that signals the auth server
// to show the code in the browser rather than redirecting. If the server
// doesn't support it, the redirect will go to this URL with ?code=... and
// the user can copy the code from the address bar.
const oobRedirectURI = "http://localhost/callback"

// OAuthLogin prepares the OAuth authorization flow:
// 1. Registers a client via Dynamic Client Registration
// 2. Generates PKCE verifier/challenge
// 3. Returns the authorization URL and an Exchange function
//
// No local HTTP server is started. The user authorizes in the browser,
// then copies the authorization code from the redirect URL and pastes it
// into the terminal.
func OAuthLogin(ctx context.Context) (*LoginResult, error) {
	redirectURI := oobRedirectURI

	// Register client via Dynamic Client Registration.
	reg, err := RegisterClient(ctx, redirectURI)
	if err != nil {
		return nil, err
	}

	// Generate PKCE code verifier and challenge.
	verifier, err := generateCodeVerifier()
	if err != nil {
		return nil, fmt.Errorf("generate PKCE verifier: %w", err)
	}
	challenge := codeChallenge(verifier)

	// Build authorization URL with PKCE.
	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {reg.ClientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {"openid profile email offline_access"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	authorizationURL := granolaAuthorizationURL + "?" + params.Encode()

	exchangeFn := func(ctx context.Context, code string) (*OAuthToken, error) {
		return exchangeCode(ctx, reg.ClientID, reg.ClientSecret, code, redirectURI, verifier)
	}

	return &LoginResult{
		AuthURL:      authorizationURL,
		ClientID:     reg.ClientID,
		ClientSecret: reg.ClientSecret,
		RedirectURI:  redirectURI,
		Exchange:     exchangeFn,
	}, nil
}

// ExtractCodeFromURL extracts the "code" query parameter from a redirect URL.
// Accepts either a full URL or just the code value.
func ExtractCodeFromURL(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("empty input")
	}

	// If it looks like a URL, parse the code parameter.
	if strings.Contains(input, "?") || strings.Contains(input, "://") {
		u, err := url.Parse(input)
		if err != nil {
			return "", fmt.Errorf("parse URL: %w", err)
		}
		code := u.Query().Get("code")
		if code == "" {
			return "", fmt.Errorf("no 'code' parameter in URL")
		}
		return code, nil
	}

	// Otherwise treat the input as the raw code.
	return input, nil
}

// exchangeCode exchanges an authorization code for an access token.
func exchangeCode(ctx context.Context, clientID, clientSecret, code, redirectURI, codeVerifier string) (*OAuthToken, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {clientID},
		"code_verifier": {codeVerifier},
	}
	if clientSecret != "" {
		data.Set("client_secret", clientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, granolaTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("token exchange: HTTP %d: %s", resp.StatusCode, string(b))
	}

	var tok OAuthToken
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}
	return &tok, nil
}
