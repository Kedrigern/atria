package users_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/joho/godotenv"

	"atria/internal/core"
	"atria/internal/database"
	"atria/internal/users"
)

func setupTestDB(t *testing.T) *sql.DB {
	_ = godotenv.Load("../../.env")

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set in .env. Skipping user integration tests.")
	}

	db, err := database.InitDB(dsn)
	if err != nil {
		t.Fatalf("Failed to connect to test db: %v", err)
	}

	_ = database.ResetDB(db)
	_ = database.MigrateUp(db)

	return db
}

func TestUserLifecycle(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// --- 1. Create a User ---
	email := "test@atria.local"
	user, err := users.CreateUser(ctx, db, email, "Test User", "password123", core.RoleUser)
	if err != nil {
		t.Fatalf("Expected no error on user creation, got: %v", err)
	}
	if user.Email != email {
		t.Errorf("Expected email %s, got %s", email, user.Email)
	}

	// --- 2. Get the User (by email) ---
	// CHANGED: calling core.FindUser instead of users.GetUser
	fetchedUser, err := core.FindUser(ctx, db, email)
	if err != nil {
		t.Fatalf("Expected to find the created user by email, got error: %v", err)
	}
	if fetchedUser.ID != user.ID {
		t.Errorf("Expected fetched ID %s to match created ID %s", fetchedUser.ID, user.ID)
	}

	// --- 3. Get the User (by UUID) ---
	// CHANGED: calling core.FindUser instead of users.GetUser
	fetchedByID, err := core.FindUser(ctx, db, user.ID.String())
	if err != nil {
		t.Fatalf("Expected to find the created user by UUID, got error: %v", err)
	}
	if fetchedByID.Email != email {
		t.Errorf("Expected fetched user to have email %s, got %s", email, fetchedByID.Email)
	}

	// --- 4. Update Role ---
	err = users.UpdateUserRole(ctx, db, email, core.RoleAdmin)
	if err != nil {
		t.Fatalf("Expected no error on role update, got: %v", err)
	}

	// CHANGED: calling core.FindUser instead of users.GetUser
	adminUser, _ := core.FindUser(ctx, db, email)
	if adminUser.Role != core.RoleAdmin {
		t.Errorf("Expected user role to be 'admin', got '%s'", adminUser.Role)
	}

	// --- 5. List Users ---
	_, _ = users.CreateUser(ctx, db, "second@atria.local", "Second User", "pass", core.RoleUser)

	allUsers, err := users.ListUsers(ctx, db)
	if err != nil {
		t.Fatalf("Expected no error when listing users, got: %v", err)
	}
	if len(allUsers) != 2 {
		t.Errorf("Expected exactly 2 users in the database, found %d", len(allUsers))
	}
}
