package main

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/vancanhuit/greenlight/internal/data"
)

func TestCreateAuthenticationTokenHandler(t *testing.T) {
	_, fakes := newTestApp(t)
	user := &data.User{ID: 1, Name: "Alice", Email: "alice@example.com", Activated: true}
	if err := user.Password.Set("password123"); err != nil {
		t.Fatalf("set password: %v", err)
	}
	fakes.users.add(user)

	reqBody := `{"email":"alice@example.com","password":"password123"}`
	res, resBody := doRequest(t, http.MethodPost, "/v1/tokens/authentication", "", bytes.NewBufferString(reqBody))

	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status: got %d want %d (body: %s)", res.StatusCode, http.StatusCreated, resBody)
	}
	var env struct {
		AuthenticationToken struct {
			Token string `json:"token"`
		} `json:"authentication_token"`
	}
	mustUnmarshal(t, resBody, &env)
	if env.AuthenticationToken.Token == "" {
		t.Error("expected a non-empty authentication token")
	}
}

func TestCreateAuthenticationTokenHandlerBadPassword(t *testing.T) {
	_, fakes := newTestApp(t)
	user := &data.User{ID: 1, Name: "Alice", Email: "alice@example.com", Activated: true}
	if err := user.Password.Set("password123"); err != nil {
		t.Fatalf("set password: %v", err)
	}
	fakes.users.add(user)

	reqBody := `{"email":"alice@example.com","password":"wrongpassword"}`
	res, resBody := doRequest(t, http.MethodPost, "/v1/tokens/authentication", "", bytes.NewBufferString(reqBody))

	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status: got %d want %d (body: %s)", res.StatusCode, http.StatusUnauthorized, resBody)
	}
	if msg := mustErrorMessage(t, resBody); msg != "invalid authentication credentials" {
		t.Errorf("unexpected error message: %q", msg)
	}
}

func TestCreateAuthenticationTokenHandlerInvalidInput(t *testing.T) {
	newTestApp(t)

	res, resBody := doRequest(t, http.MethodPost, "/v1/tokens/authentication", "", bytes.NewBufferString(`{"email":"","password":""}`))

	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status: got %d want %d (body: %s)", res.StatusCode, http.StatusUnprocessableEntity, resBody)
	}
	fields := mustErrorFields(t, resBody)
	if _, ok := fields["email"]; !ok {
		t.Errorf("expected email validation error, got %v", fields)
	}
	if _, ok := fields["password"]; !ok {
		t.Errorf("expected password validation error, got %v", fields)
	}
}
