package data_test

import (
	"errors"
	"testing"

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
