package main

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/vancanhuit/greenlight/internal/data"
)

func TestRegisterUserHandler(t *testing.T) {
	app, fakes := newTestApp(t)

	reqBody := `{"name":"Alice","email":"alice@example.com","password":"password123"}`
	res, resBody := doRequest(t, http.MethodPost, "/v1/users", "", bytes.NewBufferString(reqBody))

	// The register handler sends the welcome email in a background goroutine.
	app.wg.Wait()

	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("status: got %d want %d (body: %s)", res.StatusCode, http.StatusAccepted, resBody)
	}

	var env struct {
		User struct {
			ID        int64  `json:"id"`
			Email     string `json:"email"`
			Activated bool   `json:"activated"`
		} `json:"user"`
	}
	mustUnmarshal(t, resBody, &env)
	if env.User.Email != "alice@example.com" || env.User.Activated {
		t.Errorf("unexpected user envelope: %+v", env.User)
	}

	// movies:read must be granted to the newly registered user.
	perms := fakes.perms.perms[env.User.ID]
	if !perms.Include("movies:read") {
		t.Errorf("expected movies:read permission for user %d, got %v", env.User.ID, perms)
	}

	// The welcome email must have been recorded by the fake emailer.
	if len(fakes.mailer.sends) != 1 {
		t.Fatalf("expected 1 recorded email, got %d", len(fakes.mailer.sends))
	}
	send := fakes.mailer.sends[0]
	if send.recipient != "alice@example.com" {
		t.Errorf("email recipient: got %q want %q", send.recipient, "alice@example.com")
	}
	if send.template != "user_welcome.tmpl" {
		t.Errorf("email template: got %q want %q", send.template, "user_welcome.tmpl")
	}
}

func TestRegisterUserHandlerDuplicateEmail(t *testing.T) {
	app, fakes := newTestApp(t)
	fakes.users.add(&data.User{ID: 1, Name: "Existing", Email: "dupe@example.com"})

	reqBody := `{"name":"Bob","email":"dupe@example.com","password":"password123"}`
	res, resBody := doRequest(t, http.MethodPost, "/v1/users", "", bytes.NewBufferString(reqBody))
	app.wg.Wait()

	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status: got %d want %d (body: %s)", res.StatusCode, http.StatusUnprocessableEntity, resBody)
	}
	fields := mustErrorFields(t, resBody)
	if _, ok := fields["email"]; !ok {
		t.Errorf("expected email validation error, got %v", fields)
	}
	if len(fakes.mailer.sends) != 0 {
		t.Error("expected no email to be sent on duplicate registration")
	}
}

func TestActivateUserHandler(t *testing.T) {
	_, fakes := newTestApp(t)
	user := &data.User{ID: 1, Name: "Alice", Email: "alice@example.com", Activated: false, Version: 1}
	fakes.users.add(user)
	fakes.users.byToken[data.ScopeActivation+":"+testToken26] = user

	reqBody := `{"token":"` + testToken26 + `"}`
	res, resBody := doRequest(t, http.MethodPut, "/v1/users/activated", "", bytes.NewBufferString(reqBody))

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d (body: %s)", res.StatusCode, http.StatusOK, resBody)
	}
	var env struct {
		User struct {
			Activated bool `json:"activated"`
		} `json:"user"`
	}
	mustUnmarshal(t, resBody, &env)
	if !env.User.Activated {
		t.Error("expected user to be activated in the response envelope")
	}
	if !user.Activated {
		t.Error("expected user to be activated in the store")
	}
}

func TestActivateUserHandlerInvalidToken(t *testing.T) {
	newTestApp(t)

	// Valid length token, but not seeded, so GetForToken returns ErrRecordNotFound.
	reqBody := `{"token":"` + testToken26 + `"}`
	res, resBody := doRequest(t, http.MethodPut, "/v1/users/activated", "", bytes.NewBufferString(reqBody))

	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status: got %d want %d (body: %s)", res.StatusCode, http.StatusUnprocessableEntity, resBody)
	}
	fields := mustErrorFields(t, resBody)
	if fields["token"] != "invalid or expired activation token" {
		t.Errorf("unexpected token error: %v", fields)
	}
}

func TestActivateUserHandlerMalformedToken(t *testing.T) {
	newTestApp(t)

	res, resBody := doRequest(t, http.MethodPut, "/v1/users/activated", "", bytes.NewBufferString(`{"token":"too-short"}`))

	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status: got %d want %d (body: %s)", res.StatusCode, http.StatusUnprocessableEntity, resBody)
	}
	fields := mustErrorFields(t, resBody)
	if _, ok := fields["token"]; !ok {
		t.Errorf("expected token validation error, got %v", fields)
	}
}
