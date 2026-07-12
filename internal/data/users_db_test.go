package data_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vancanhuit/greenlight/internal/data"
)

func newTestUser(t *testing.T, email string) *data.User {
	t.Helper()
	u := &data.User{Name: "Alice", Email: email, Activated: true}
	if err := u.Password.Set("password123"); err != nil {
		t.Fatalf("set password: %v", err)
	}
	return u
}

func TestUserInsertDuplicateEmail(t *testing.T) {
	requireDB(t)
	truncate(t, "users")

	models := data.NewModels(testPool)
	if err := models.Users.Insert(newTestUser(t, "a@example.com")); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	err := models.Users.Insert(newTestUser(t, "a@example.com"))
	if !errors.Is(err, data.ErrDuplicateEmail) {
		t.Fatalf("expected ErrDuplicateEmail, got %v", err)
	}
}

func TestUserGetByEmail(t *testing.T) {
	requireDB(t)
	truncate(t, "users")

	models := data.NewModels(testPool)
	u := newTestUser(t, "b@example.com")
	if err := models.Users.Insert(u); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := models.Users.GetByEmail("b@example.com")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Alice" || !got.Activated {
		t.Fatalf("unexpected user: %+v", got)
	}
}

func TestRegisterUserAtomic(t *testing.T) {
	requireDB(t)
	truncate(t, "users")

	models := data.NewModels(testPool)
	user := newTestUser(t, "reg@example.com")
	user.Activated = false

	token, err := models.RegisterUser(user, []string{"movies:read"}, time.Hour, data.ScopeActivation)
	if err != nil {
		t.Fatalf("RegisterUser: %v", err)
	}
	if user.ID == 0 {
		t.Fatal("expected user ID to be set")
	}
	if token == nil || token.Plaintext == "" {
		t.Fatal("expected a token to be returned")
	}

	perms, err := models.Permissions.GetAllForUser(user.ID)
	if err != nil {
		t.Fatalf("GetAllForUser: %v", err)
	}
	if !perms.Include("movies:read") {
		t.Errorf("expected movies:read permission, got %v", perms)
	}

	// The activation token must resolve back to the same user.
	got, err := models.Users.GetForToken(data.ScopeActivation, token.Plaintext)
	if err != nil {
		t.Fatalf("GetForToken: %v", err)
	}
	if got.ID != user.ID {
		t.Errorf("token maps to user %d, want %d", got.ID, user.ID)
	}
}

func TestRegisterUserRollbackOnDuplicate(t *testing.T) {
	requireDB(t)
	truncate(t, "users")

	models := data.NewModels(testPool)
	first := newTestUser(t, "dupe@example.com")
	if _, err := models.RegisterUser(first, []string{"movies:read"}, time.Hour, data.ScopeActivation); err != nil {
		t.Fatalf("first RegisterUser: %v", err)
	}

	dup := newTestUser(t, "dupe@example.com")
	_, err := models.RegisterUser(dup, []string{"movies:read"}, time.Hour, data.ScopeActivation)
	if !errors.Is(err, data.ErrDuplicateEmail) {
		t.Fatalf("expected ErrDuplicateEmail, got %v", err)
	}

	// The failed registration must not have created a second user row.
	var count int
	if err := testPool.QueryRow(context.Background(), "SELECT count(*) FROM users").Scan(&count); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 user after rolled-back duplicate, got %d", count)
	}
}

func TestActivateUserAtomic(t *testing.T) {
	requireDB(t)
	truncate(t, "users")

	models := data.NewModels(testPool)
	user := newTestUser(t, "activate@example.com")
	user.Activated = false

	token, err := models.RegisterUser(user, []string{"movies:read"}, time.Hour, data.ScopeActivation)
	if err != nil {
		t.Fatalf("RegisterUser: %v", err)
	}

	user.Activated = true
	if err := models.ActivateUser(user, data.ScopeActivation); err != nil {
		t.Fatalf("ActivateUser: %v", err)
	}

	// Activation tokens must be deleted, so the token no longer resolves.
	if _, err := models.Users.GetForToken(data.ScopeActivation, token.Plaintext); !errors.Is(err, data.ErrRecordNotFound) {
		t.Fatalf("expected token to be deleted (ErrRecordNotFound), got %v", err)
	}

	// The user must be persisted as activated.
	got, err := models.Users.GetByEmail("activate@example.com")
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if !got.Activated {
		t.Error("expected user to be activated in the store")
	}
}

func TestActivateUserEditConflict(t *testing.T) {
	requireDB(t)
	truncate(t, "users")

	models := data.NewModels(testPool)
	user := newTestUser(t, "conflict@example.com")
	user.Activated = false

	if _, err := models.RegisterUser(user, []string{"movies:read"}, time.Hour, data.ScopeActivation); err != nil {
		t.Fatalf("RegisterUser: %v", err)
	}

	// Simulate a stale write: bump the in-memory version past the stored row.
	user.Version++
	user.Activated = true
	if err := models.ActivateUser(user, data.ScopeActivation); !errors.Is(err, data.ErrEditConflict) {
		t.Fatalf("expected ErrEditConflict, got %v", err)
	}

	// The rolled-back activation must not have persisted the activated state.
	got, err := models.Users.GetByEmail("conflict@example.com")
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if got.Activated {
		t.Error("expected user to remain unactivated after edit conflict")
	}
}

func TestRegisterUserNoPermissions(t *testing.T) {
	requireDB(t)
	truncate(t, "users")

	models := data.NewModels(testPool)
	user := newTestUser(t, "noperm@example.com")
	user.Activated = false

	token, err := models.RegisterUser(user, nil, time.Hour, data.ScopeActivation)
	if err != nil {
		t.Fatalf("RegisterUser: %v", err)
	}
	if user.ID == 0 {
		t.Fatal("expected user ID to be set")
	}
	if token == nil || token.Plaintext == "" {
		t.Fatal("expected a token to be returned")
	}

	perms, err := models.Permissions.GetAllForUser(user.ID)
	if err != nil {
		t.Fatalf("GetAllForUser: %v", err)
	}
	if len(perms) != 0 {
		t.Errorf("expected no permissions, got %v", perms)
	}
}

func TestGetForTokenExpired(t *testing.T) {
	requireDB(t)
	truncate(t, "users")

	models := data.NewModels(testPool)
	user := newTestUser(t, "expired@example.com")
	if err := models.Users.Insert(user); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// A token that expired an hour ago must not resolve to its user.
	token, err := models.Tokens.New(user.ID, -time.Hour, data.ScopeActivation)
	if err != nil {
		t.Fatalf("Tokens.New: %v", err)
	}

	if _, err := models.Users.GetForToken(data.ScopeActivation, token.Plaintext); !errors.Is(err, data.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound for expired token, got %v", err)
	}
}
