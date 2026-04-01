package auth

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"connectrpc.com/connect"
	qrcode "github.com/skip2/go-qrcode"

	blackwoodv1 "github.com/csweichel/blackwood/gen/blackwood/v1"
	"github.com/csweichel/blackwood/internal/storage"
)

// Handler implements the AuthService Connect handler.
type Handler struct {
	store       *storage.Store
	rateLimiter *RateLimiter
	useTLS      bool
}

// NewHandler creates an auth Connect service handler.
func NewHandler(store *storage.Store, rateLimiter *RateLimiter, useTLS bool) *Handler {
	return &Handler{
		store:       store,
		rateLimiter: rateLimiter,
		useTLS:      useTLS,
	}
}

func (h *Handler) Status(ctx context.Context, req *connect.Request[blackwoodv1.AuthStatusRequest]) (*connect.Response[blackwoodv1.AuthStatusResponse], error) {
	secret, err := h.store.GetTOTPSecret(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("check TOTP secret: %w", err))
	}

	token := getSessionTokenFromHeader(req.Header())
	authenticated := ValidateSession(ctx, h.store, token)

	return connect.NewResponse(&blackwoodv1.AuthStatusResponse{
		Authenticated: authenticated,
		SetupRequired: secret == "",
	}), nil
}

func (h *Handler) GetSetupInfo(ctx context.Context, req *connect.Request[blackwoodv1.GetSetupInfoRequest]) (*connect.Response[blackwoodv1.GetSetupInfoResponse], error) {
	existing, err := h.store.GetTOTPSecret(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("check TOTP secret: %w", err))
	}
	if existing != "" {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("setup already complete"))
	}

	secret, uri, err := GenerateSecret()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("generate TOTP secret: %w", err))
	}

	qrPNG, err := qrcode.Encode(uri, qrcode.Medium, 200)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("generate QR code: %w", err))
	}

	return connect.NewResponse(&blackwoodv1.GetSetupInfoResponse{
		Secret: secret,
		QrCode: base64.StdEncoding.EncodeToString(qrPNG),
	}), nil
}

func (h *Handler) ConfirmSetup(ctx context.Context, req *connect.Request[blackwoodv1.ConfirmSetupRequest]) (*connect.Response[blackwoodv1.ConfirmSetupResponse], error) {
	existing, err := h.store.GetTOTPSecret(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("check TOTP secret: %w", err))
	}
	if existing != "" {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("setup already complete"))
	}

	msg := req.Msg
	if msg.Secret == "" || msg.Code == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("secret and code are required"))
	}

	if !ValidateCode(msg.Secret, msg.Code) {
		return connect.NewResponse(&blackwoodv1.ConfirmSetupResponse{
			Ok:    false,
			Error: "Invalid code. Please try again.",
		}), nil
	}

	if err := h.store.SaveTOTPSecret(ctx, msg.Secret); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("save TOTP secret: %w", err))
	}

	slog.Info("TOTP setup complete")

	return connect.NewResponse(&blackwoodv1.ConfirmSetupResponse{Ok: true}), nil
}

func (h *Handler) Login(ctx context.Context, req *connect.Request[blackwoodv1.LoginRequest]) (*connect.Response[blackwoodv1.LoginResponse], error) {
	ip := clientIPFromHeader(req.Header(), req.Peer().Addr)

	if !h.rateLimiter.Allow(ip) {
		return nil, connect.NewError(connect.CodeResourceExhausted, fmt.Errorf("too many attempts, try again later"))
	}

	if req.Msg.Code == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("code is required"))
	}

	secret, err := h.store.GetTOTPSecret(ctx)
	if err != nil || secret == "" {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read TOTP secret: %w", err))
	}

	if !ValidateCode(secret, req.Msg.Code) {
		h.rateLimiter.RecordFailure(ip)
		return connect.NewResponse(&blackwoodv1.LoginResponse{
			Ok:    false,
			Error: "Invalid code. Please try again.",
		}), nil
	}

	// Success — reset rate limiter and create session.
	h.rateLimiter.Reset(ip)
	_ = h.store.CleanExpiredSessions(ctx)

	token, err := CreateSession(ctx, h.store)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create session: %w", err))
	}

	resp := connect.NewResponse(&blackwoodv1.LoginResponse{Ok: true})
	cookie := sessionCookie(token, h.useTLS)
	resp.Header().Set("Set-Cookie", cookie.String())
	return resp, nil
}

func (h *Handler) Logout(ctx context.Context, req *connect.Request[blackwoodv1.LogoutRequest]) (*connect.Response[blackwoodv1.LogoutResponse], error) {
	token := getSessionTokenFromHeader(req.Header())
	if token != "" {
		_ = h.store.DeleteSession(ctx, token)
	}

	resp := connect.NewResponse(&blackwoodv1.LogoutResponse{})
	cookie := clearCookie()
	resp.Header().Set("Set-Cookie", cookie.String())
	return resp, nil
}

// sessionCookie builds the session cookie value.
func sessionCookie(token string, useTLS bool) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(SessionLifetime.Seconds()),
		HttpOnly: true,
		Secure:   useTLS,
		SameSite: http.SameSiteLaxMode,
	}
}

// clearCookie builds a cookie that clears the session.
func clearCookie() *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

// getSessionTokenFromHeader extracts the session token from the Cookie header.
func getSessionTokenFromHeader(h http.Header) string {
	// Parse the Cookie header manually to extract our session token.
	r := &http.Request{Header: h}
	c, err := r.Cookie(SessionCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

// clientIPFromHeader extracts the client IP, preferring X-Forwarded-For.
func clientIPFromHeader(h http.Header, peerAddr string) string {
	if xff := h.Get("X-Forwarded-For"); xff != "" {
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return trimSpace(xff[:i])
			}
		}
		return trimSpace(xff)
	}
	host, _, err := net.SplitHostPort(peerAddr)
	if err != nil {
		return peerAddr
	}
	return host
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && s[start] == ' ' {
		start++
	}
	for end > start && s[end-1] == ' ' {
		end--
	}
	return s[start:end]
}
