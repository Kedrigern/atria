package users_test

import (
	"context"
	"testing"

	"atria/internal/core"
	"atria/internal/testutil"
	"atria/internal/users"
)

func TestUserLifecycle(t *testing.T) {
	db, _ := testutil.SetupTestDB(t)
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
	fetchedUser, err := core.FindUser(ctx, db, email)
	if err != nil {
		t.Fatalf("Expected to find the created user by email, got error: %v", err)
	}
	if fetchedUser.ID != user.ID {
		t.Errorf("Expected fetched ID %s to match created ID %s", fetchedUser.ID, user.ID)
	}

	// --- 3. Get the User (by UUID) ---
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
	if len(allUsers) != 3 {
		t.Errorf("Expected exactly 2 users in the database, found %d", len(allUsers))
	}
}
