// Package auth provides authentication middleware, password hashing, and
// session token management for the Codex server.
package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/scootsy/library-server/internal/database/queries"
	"golang.org/x/crypto/bcrypt"
)

type contextKey string

const (
	// UserContextKey is the context key for the authenticated user.
	UserContextKey contextKey = "auth_user"

	// SessionDuration is how long a session token remains valid.
	SessionDuration = 30 * 24 * time.Hour // 30 days

	// bcryptCost is the cost factor for bcrypt hashing (CLAUDE.md: >= 12).
	bcryptCost = 12

	// tokenBytes is the number of random bytes for session tokens.
	tokenBytes = 32
)

// HashPassword hashes a plaintext password using bcrypt with cost >= 12.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hashing password: %w", err)
	}
	return string(hash), nil
}

// CheckPassword compares a plaintext password against a bcrypt hash.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// GenerateSessionToken creates a cryptographically random session token.
func GenerateSessionToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating session token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// UserFromContext extracts the authenticated user from the request context.
func UserFromContext(ctx context.Context) *queries.User {
	u, _ := ctx.Value(UserContextKey).(*queries.User)
	return u
}

// Middleware returns HTTP middleware that requires a valid session.
// It checks for a session token in the Authorization header (Bearer) or
// a cookie named "codex_session".
func Middleware(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r)
			if token == "" {
				http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
				return
			}

			session, err := queries.GetSessionByID(db, token)
			if err != nil {
				slog.Error("session lookup failed", "error", err)
				http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
				return
			}
			if session == nil {
				http.Error(w, `{"error":"invalid or expired session"}`, http.StatusUnauthorized)
				return
			}

			// Check expiry
			expiresAt, err := time.Parse(time.RFC3339, session.ExpiresAt)
			if err != nil {
				slog.Error("invalid session expiry format", "session_id", session.ID, "error", err)
				http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
				return
			}
			if time.Now().UTC().After(expiresAt) {
				_ = queries.DeleteSession(db, session.ID)
				http.Error(w, `{"error":"session expired"}`, http.StatusUnauthorized)
				return
			}

			user, err := queries.GetUserByID(db, session.UserID)
			if err != nil {
				slog.Error("user lookup failed", "user_id", session.UserID, "error", err)
				http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
				return
			}
			if user == nil || !user.IsActive {
				http.Error(w, `{"error":"account disabled"}`, http.StatusForbidden)
				return
			}

			// Update session last_used_at (best-effort)
			_ = queries.TouchSession(db, session.ID)

			ctx := context.WithValue(r.Context(), UserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole returns middleware that checks the user has one of the given roles.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := UserFromContext(r.Context())
			if user == nil {
				http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
				return
			}
			if !allowed[user.Role] {
				http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// extractToken gets the session token from Authorization header or cookie.
func extractToken(r *http.Request) string {
	// Check Authorization: Bearer <token>
	if auth := r.Header.Get("Authorization"); auth != "" {
		if strings.HasPrefix(auth, "Bearer ") {
			return strings.TrimPrefix(auth, "Bearer ")
		}
	}

	// Check cookie
	cookie, err := r.Cookie("codex_session")
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}

	return ""
}
