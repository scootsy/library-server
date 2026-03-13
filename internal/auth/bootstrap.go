package auth

import (
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/scootsy/library-server/internal/database/queries"
)

// EnsureAdminUser creates the initial admin account if no users exist yet.
// The password parameter is the plaintext password from config/env.
func EnsureAdminUser(db *sql.DB, password string) error {
	count, err := queries.CountUsers(db)
	if err != nil {
		return fmt.Errorf("counting users: %w", err)
	}
	if count > 0 {
		return nil // users already exist
	}

	hash, err := HashPassword(password)
	if err != nil {
		return fmt.Errorf("hashing admin password: %w", err)
	}

	user := &queries.User{
		ID:           uuid.NewString(),
		Username:     "admin",
		DisplayName:  "Administrator",
		Role:         "admin",
		IsActive:     true,
		PasswordHash: hash,
	}

	if err := queries.CreateUser(db, user); err != nil {
		return fmt.Errorf("creating admin user: %w", err)
	}

	slog.Info("created initial admin user", "username", "admin")
	return nil
}
