package queries

import (
	"database/sql"
	"fmt"
	"time"
)

// User represents a row in the users table.
type User struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"display_name,omitempty"`
	Email       string  `json:"email,omitempty"`
	Role        string  `json:"role"`
	IsActive    bool    `json:"is_active"`
	CreatedAt   string  `json:"created_at"`
	LastLoginAt *string `json:"last_login_at,omitempty"`

	// PasswordHash is never serialized to JSON.
	PasswordHash string `json:"-"`
}

// Session represents a row in the sessions table.
type Session struct {
	ID         string  `json:"id"`
	UserID     string  `json:"user_id"`
	DeviceName string  `json:"device_name,omitempty"`
	DeviceType string  `json:"device_type,omitempty"`
	CreatedAt  string  `json:"created_at"`
	ExpiresAt  string  `json:"expires_at"`
	LastUsedAt *string `json:"last_used_at,omitempty"`
}

// CreateUser inserts a new user.
func CreateUser(db *sql.DB, u *User) error {
	_, err := db.Exec(`INSERT INTO users (id, username, display_name, email, password_hash, role, is_active)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.Username, u.DisplayName, u.Email, u.PasswordHash, u.Role, u.IsActive)
	if err != nil {
		return fmt.Errorf("inserting user %q: %w", u.Username, err)
	}
	return nil
}

// GetUserByID retrieves a user by primary key.
func GetUserByID(db *sql.DB, id string) (*User, error) {
	row := db.QueryRow(`SELECT id, username, display_name, email, password_hash, role, is_active, created_at, last_login_at
		FROM users WHERE id = ?`, id)
	return scanUser(row)
}

// GetUserByUsername retrieves a user by username.
func GetUserByUsername(db *sql.DB, username string) (*User, error) {
	row := db.QueryRow(`SELECT id, username, display_name, email, password_hash, role, is_active, created_at, last_login_at
		FROM users WHERE username = ?`, username)
	return scanUser(row)
}

// ListUsers returns all users.
func ListUsers(db *sql.DB) ([]*User, error) {
	rows, err := db.Query(`SELECT id, username, display_name, email, password_hash, role, is_active, created_at, last_login_at
		FROM users ORDER BY username`)
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u, err := scanUserRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning user row: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// UpdateUser updates mutable user fields.
func UpdateUser(db *sql.DB, u *User) error {
	_, err := db.Exec(`UPDATE users SET username = ?, display_name = ?, email = ?, role = ?, is_active = ? WHERE id = ?`,
		u.Username, u.DisplayName, u.Email, u.Role, u.IsActive, u.ID)
	if err != nil {
		return fmt.Errorf("updating user %q: %w", u.ID, err)
	}
	return nil
}

// UpdateUserPassword sets the password hash for a user.
func UpdateUserPassword(db *sql.DB, userID, hash string) error {
	_, err := db.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, hash, userID)
	if err != nil {
		return fmt.Errorf("updating password for user %q: %w", userID, err)
	}
	return nil
}

// UpdateUserLastLogin sets the last_login_at timestamp.
func UpdateUserLastLogin(db *sql.DB, userID string) error {
	_, err := db.Exec(`UPDATE users SET last_login_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), userID)
	if err != nil {
		return fmt.Errorf("updating last login for user %q: %w", userID, err)
	}
	return nil
}

// DeleteUser removes a user by ID.
func DeleteUser(db *sql.DB, id string) error {
	_, err := db.Exec(`DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting user %q: %w", id, err)
	}
	return nil
}

// CountUsers returns the total number of users.
func CountUsers(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

// ── Sessions ─────────────────────────────────────────────────────────────────

// CreateSession inserts a new session.
func CreateSession(db *sql.DB, s *Session) error {
	_, err := db.Exec(`INSERT INTO sessions (id, user_id, device_name, device_type, expires_at)
		VALUES (?, ?, ?, ?, ?)`,
		s.ID, s.UserID, s.DeviceName, s.DeviceType, s.ExpiresAt)
	if err != nil {
		return fmt.Errorf("inserting session: %w", err)
	}
	return nil
}

// GetSessionByID retrieves a session by its token ID.
func GetSessionByID(db *sql.DB, id string) (*Session, error) {
	row := db.QueryRow(`SELECT id, user_id, device_name, device_type, created_at, expires_at, last_used_at
		FROM sessions WHERE id = ?`, id)
	var s Session
	var deviceName, deviceType sql.NullString
	var lastUsedAt sql.NullString
	err := row.Scan(&s.ID, &s.UserID, &deviceName, &deviceType, &s.CreatedAt, &s.ExpiresAt, &lastUsedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scanning session: %w", err)
	}
	s.DeviceName = deviceName.String
	s.DeviceType = deviceType.String
	if lastUsedAt.Valid {
		s.LastUsedAt = &lastUsedAt.String
	}
	return &s, nil
}

// TouchSession updates the last_used_at timestamp for a session.
func TouchSession(db *sql.DB, sessionID string) error {
	_, err := db.Exec(`UPDATE sessions SET last_used_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), sessionID)
	return err
}

// DeleteSession removes a session by ID.
func DeleteSession(db *sql.DB, id string) error {
	_, err := db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

// DeleteExpiredSessions removes all sessions past their expiry time.
func DeleteExpiredSessions(db *sql.DB) (int64, error) {
	result, err := db.Exec(`DELETE FROM sessions WHERE expires_at < ?`,
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("deleting expired sessions: %w", err)
	}
	return result.RowsAffected()
}

// DeleteUserSessions removes all sessions for a given user.
func DeleteUserSessions(db *sql.DB, userID string) error {
	_, err := db.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

// ── helpers ──────────────────────────────────────────────────────────────────

func scanUser(row *sql.Row) (*User, error) {
	var u User
	var displayName, email sql.NullString
	var passwordHash sql.NullString
	var lastLoginAt sql.NullString
	err := row.Scan(&u.ID, &u.Username, &displayName, &email, &passwordHash, &u.Role, &u.IsActive, &u.CreatedAt, &lastLoginAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scanning user: %w", err)
	}
	u.DisplayName = displayName.String
	u.Email = email.String
	u.PasswordHash = passwordHash.String
	if lastLoginAt.Valid {
		u.LastLoginAt = &lastLoginAt.String
	}
	return &u, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUserRow(row rowScanner) (*User, error) {
	var u User
	var displayName, email sql.NullString
	var passwordHash sql.NullString
	var lastLoginAt sql.NullString
	err := row.Scan(&u.ID, &u.Username, &displayName, &email, &passwordHash, &u.Role, &u.IsActive, &u.CreatedAt, &lastLoginAt)
	if err != nil {
		return nil, err
	}
	u.DisplayName = displayName.String
	u.Email = email.String
	u.PasswordHash = passwordHash.String
	if lastLoginAt.Valid {
		u.LastLoginAt = &lastLoginAt.String
	}
	return &u, nil
}
