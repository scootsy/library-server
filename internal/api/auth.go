package api

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scootsy/library-server/internal/auth"
	"github.com/scootsy/library-server/internal/config"
	"github.com/scootsy/library-server/internal/database/queries"
)

// AuthHandler handles authentication endpoints (login, logout, current user).
type AuthHandler struct {
	db     *sql.DB
	config *config.Config
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string       `json:"token"`
	User  *queries.User `json:"user"`
}

// Login authenticates a user with username/password and returns a session token.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	user, err := queries.GetUserByUsername(h.db, req.Username)
	if err != nil {
		slog.Error("login: user lookup failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user == nil || !user.IsActive {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if user.PasswordHash == "" || !auth.CheckPassword(user.PasswordHash, req.Password) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// Generate session token
	token, err := auth.GenerateSessionToken()
	if err != nil {
		slog.Error("login: failed to generate session token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	lifetimeDays := h.config.Auth.SessionLifetimeDays
	if lifetimeDays <= 0 {
		lifetimeDays = 30
	}
	expiresAt := time.Now().UTC().Add(time.Duration(lifetimeDays) * 24 * time.Hour)

	session := &queries.Session{
		ID:        token,
		UserID:    user.ID,
		ExpiresAt: expiresAt.Format(time.RFC3339),
	}
	if err := queries.CreateSession(h.db, session); err != nil {
		slog.Error("login: failed to create session", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := queries.UpdateUserLastLogin(h.db, user.ID); err != nil {
		slog.Warn("login: failed to update last_login_at", "error", err)
	}

	// Set session cookie. Derive the Secure flag from the configured base_url
	// so the cookie works correctly behind HTTPS reverse proxies.
	secureCookie := strings.HasPrefix(h.config.Server.BaseURL, "https://")
	http.SetCookie(w, &http.Cookie{
		Name:     "codex_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secureCookie,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   lifetimeDays * 24 * 60 * 60,
	})

	writeJSON(w, http.StatusOK, loginResponse{Token: token, User: user})
}

// Logout invalidates the current session.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Extract the token to delete the specific session
	token := ""
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		if len(authHeader) > 7 {
			token = authHeader[7:]
		}
	}
	if token == "" {
		if cookie, err := r.Cookie("codex_session"); err == nil {
			token = cookie.Value
		}
	}

	if token != "" {
		if err := queries.DeleteSession(h.db, token); err != nil {
			slog.Error("logout: failed to delete session", "error", err)
		}
	}

	// Clear cookie. Secure flag must match the login cookie for browsers to clear it.
	http.SetCookie(w, &http.Cookie{
		Name:     "codex_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   strings.HasPrefix(h.config.Server.BaseURL, "https://"),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

// Me returns the currently authenticated user.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// ── User management (admin-only) ────────────────────────────────────────────

// UsersHandler handles admin user management endpoints.
type UsersHandler struct {
	db *sql.DB
}

// ListUsers returns all users (admin only).
func (h *UsersHandler) List(w http.ResponseWriter, r *http.Request) {
	users, err := queries.ListUsers(h.db)
	if err != nil {
		slog.Error("failed to list users", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": users})
}

// GetUser returns a single user (admin only).
func (h *UsersHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	user, err := queries.GetUserByID(h.db, id)
	if err != nil {
		slog.Error("failed to get user", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get user")
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

type createUserRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	Role        string `json:"role"`
}

// CreateUser creates a new user (admin only).
func (h *UsersHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}
	if req.Password == "" {
		writeError(w, http.StatusBadRequest, "password is required")
		return
	}
	if req.Role == "" {
		req.Role = "user"
	}
	if req.Role != "admin" && req.Role != "user" && req.Role != "guest" {
		writeError(w, http.StatusBadRequest, "role must be admin, user, or guest")
		return
	}

	// Check for duplicate username
	existing, err := queries.GetUserByUsername(h.db, req.Username)
	if err != nil {
		slog.Error("user lookup failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if existing != nil {
		writeError(w, http.StatusConflict, "username already exists")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		slog.Error("failed to hash password", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	user := &queries.User{
		ID:           uuid.NewString(),
		Username:     req.Username,
		DisplayName:  req.DisplayName,
		Email:        req.Email,
		PasswordHash: hash,
		Role:         req.Role,
		IsActive:     true,
	}

	if err := queries.CreateUser(h.db, user); err != nil {
		slog.Error("failed to create user", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	writeJSON(w, http.StatusCreated, user)
}

type updateUserRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Role        string `json:"role"`
	IsActive    *bool  `json:"is_active"`
	Password    string `json:"password"`
}

// UpdateUser modifies a user (admin only).
func (h *UsersHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	user, err := queries.GetUserByID(h.db, id)
	if err != nil {
		slog.Error("failed to get user", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Username != "" {
		user.Username = req.Username
	}
	if req.DisplayName != "" {
		user.DisplayName = req.DisplayName
	}
	user.Email = req.Email
	if req.Role != "" {
		if req.Role != "admin" && req.Role != "user" && req.Role != "guest" {
			writeError(w, http.StatusBadRequest, "role must be admin, user, or guest")
			return
		}
		user.Role = req.Role
	}
	if req.IsActive != nil {
		user.IsActive = *req.IsActive
	}

	if err := queries.UpdateUser(h.db, user); err != nil {
		slog.Error("failed to update user", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update user")
		return
	}

	// Update password if provided
	if req.Password != "" {
		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			slog.Error("failed to hash password", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if err := queries.UpdateUserPassword(h.db, id, hash); err != nil {
			slog.Error("failed to update password", "id", id, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to update password")
			return
		}
	}

	writeJSON(w, http.StatusOK, user)
}

// DeleteUser removes a user (admin only).
func (h *UsersHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Prevent self-deletion
	currentUser := auth.UserFromContext(r.Context())
	if currentUser != nil && currentUser.ID == id {
		writeError(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}

	if err := queries.DeleteUserSessions(h.db, id); err != nil {
		slog.Error("failed to delete user sessions", "id", id, "error", err)
	}
	if err := queries.DeleteUser(h.db, id); err != nil {
		slog.Error("failed to delete user", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ChangePassword allows the current user to change their own password.
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "current_password and new_password are required")
		return
	}

	if !auth.CheckPassword(user.PasswordHash, req.CurrentPassword) {
		writeError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		slog.Error("failed to hash new password", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := queries.UpdateUserPassword(h.db, user.ID, hash); err != nil {
		slog.Error("failed to update password", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update password")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "password changed"})
}
