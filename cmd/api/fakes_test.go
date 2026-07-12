package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/vancanhuit/greenlight/internal/data"
	"go.uber.org/goleak"
)

// testToken26 is a 26-byte plaintext token, satisfying data.ValidateTokenPlaintext.
const testToken26 = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

// --- Fakes implementing the seams in stores.go ---------------------------------

type fakeMovieStore struct {
	movies map[int64]*data.Movie
	nextID int64

	updateErr error

	// list result returned by GetAll plus the arguments it was called with.
	listMovies   []*data.Movie
	listMetadata data.Metadata
	gotTitle     string
	gotGenres    []string
	gotFilters   data.Filters
}

func newFakeMovieStore() *fakeMovieStore {
	return &fakeMovieStore{movies: map[int64]*data.Movie{}}
}

func (s *fakeMovieStore) seed(m *data.Movie) {
	s.movies[m.ID] = m
	if m.ID > s.nextID {
		s.nextID = m.ID
	}
}

func (s *fakeMovieStore) Insert(movie *data.Movie) error {
	s.nextID++
	movie.ID = s.nextID
	movie.Version = 1
	movie.CreatedAt = time.Now()
	stored := *movie
	s.movies[movie.ID] = &stored
	return nil
}

func (s *fakeMovieStore) Get(id int64) (*data.Movie, error) {
	m, ok := s.movies[id]
	if !ok {
		return nil, data.ErrRecordNotFound
	}
	c := *m
	return &c, nil
}

func (s *fakeMovieStore) GetAll(title string, genres []string, filters data.Filters) ([]*data.Movie, data.Metadata, error) {
	s.gotTitle = title
	s.gotGenres = genres
	s.gotFilters = filters
	return s.listMovies, s.listMetadata, nil
}

func (s *fakeMovieStore) Update(movie *data.Movie) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	if _, ok := s.movies[movie.ID]; !ok {
		return data.ErrEditConflict
	}
	movie.Version++
	c := *movie
	s.movies[movie.ID] = &c
	return nil
}

func (s *fakeMovieStore) Delete(id int64) error {
	if _, ok := s.movies[id]; !ok {
		return data.ErrRecordNotFound
	}
	delete(s.movies, id)
	return nil
}

type fakeUserStore struct {
	byID    map[int64]*data.User
	byEmail map[string]*data.User
	byToken map[string]*data.User
	nextID  int64
}

func newFakeUserStore() *fakeUserStore {
	return &fakeUserStore{
		byID:    map[int64]*data.User{},
		byEmail: map[string]*data.User{},
		byToken: map[string]*data.User{},
	}
}

func (s *fakeUserStore) add(u *data.User) {
	s.byID[u.ID] = u
	s.byEmail[u.Email] = u
	if u.ID > s.nextID {
		s.nextID = u.ID
	}
}

func (s *fakeUserStore) Insert(user *data.User) error {
	if _, ok := s.byEmail[user.Email]; ok {
		return data.ErrDuplicateEmail
	}
	s.nextID++
	user.ID = s.nextID
	user.Version = 1
	user.CreatedAt = time.Now()
	s.byID[user.ID] = user
	s.byEmail[user.Email] = user
	return nil
}

func (s *fakeUserStore) GetByEmail(email string) (*data.User, error) {
	u, ok := s.byEmail[email]
	if !ok {
		return nil, data.ErrRecordNotFound
	}
	return u, nil
}

func (s *fakeUserStore) Update(user *data.User) error {
	if _, ok := s.byID[user.ID]; !ok {
		return data.ErrEditConflict
	}
	user.Version++
	s.byID[user.ID] = user
	s.byEmail[user.Email] = user
	return nil
}

func (s *fakeUserStore) GetForToken(tokenScope, tokenPlaintext string) (*data.User, error) {
	u, ok := s.byToken[tokenScope+":"+tokenPlaintext]
	if !ok {
		return nil, data.ErrRecordNotFound
	}
	return u, nil
}

type fakeTokenStore struct{}

func (s *fakeTokenStore) DeleteAllForUser(scope string, userID int64) error {
	return nil
}

func (s *fakeTokenStore) New(userID int64, ttl time.Duration, scope string) (*data.Token, error) {
	token := &data.Token{
		Plaintext: testToken26,
		UserID:    userID,
		Expiry:    time.Now().Add(ttl),
		Scope:     scope,
	}
	return token, nil
}

type fakePermissionStore struct {
	perms map[int64]data.Permissions
}

func newFakePermissionStore() *fakePermissionStore {
	return &fakePermissionStore{perms: map[int64]data.Permissions{}}
}

func (s *fakePermissionStore) GetAllForUser(userID int64) (data.Permissions, error) {
	return s.perms[userID], nil
}

func (s *fakePermissionStore) AddForUser(userID int64, codes ...string) error {
	s.perms[userID] = append(s.perms[userID], codes...)
	return nil
}

type recordedEmail struct {
	recipient string
	template  string
	data      any
}

type fakeEmailer struct {
	sends []recordedEmail
}

func (e *fakeEmailer) Send(recipient, templateFile string, data any) error {
	e.sends = append(e.sends, recordedEmail{recipient: recipient, template: templateFile, data: data})
	return nil
}

// --- Test application wiring ---------------------------------------------------

type testFakes struct {
	movies *fakeMovieStore
	users  *fakeUserStore
	tokens *fakeTokenStore
	perms  *fakePermissionStore
	mailer *fakeEmailer
}

func newFakes() *testFakes {
	return &testFakes{
		movies: newFakeMovieStore(),
		users:  newFakeUserStore(),
		tokens: &fakeTokenStore{},
		perms:  newFakePermissionStore(),
		mailer: &fakeEmailer{},
	}
}

func (f *testFakes) install(app *application) {
	app.movies = f.movies
	app.users = f.users
	app.tokens = f.tokens
	app.permissions = f.perms
	app.mailer = f.mailer
}

// seedAuthedUser registers an activated user reachable via testToken26 and grants
// it the given permissions, so authenticate/requirePermissions let requests through.
func (f *testFakes) seedAuthedUser(t *testing.T, perms ...string) (*data.User, string) {
	t.Helper()
	user := &data.User{ID: 1, Name: "Auth User", Email: "auth@example.com", Activated: true, Version: 1}
	if err := user.Password.Set("password123"); err != nil {
		t.Fatalf("set password: %v", err)
	}
	f.users.add(user)
	f.users.byToken[data.ScopeAuthentication+":"+testToken26] = user
	f.perms.perms[user.ID] = append(f.perms.perms[user.ID], perms...)
	return user, testToken26
}

// The metrics middleware registers process-global expvar counters when the chi
// chain is first built, so the real router (app.routes()) is built exactly once
// per test binary. Fakes are reassigned per test for isolation; handlers read the
// interface fields off the shared *application at request time. Tests must not run
// in parallel.
var (
	testAppOnce        sync.Once
	sharedApp          *application
	sharedRouter       http.Handler
	testShutdownCancel context.CancelFunc
)

// TestMain cancels the shared app's shutdown context (stopping the rate-limiter
// cleanup goroutine it spawned) and then asserts no goroutines leaked.
func TestMain(m *testing.M) {
	code := m.Run()
	if testShutdownCancel != nil {
		testShutdownCancel()
	}
	if err := goleak.Find(); err != nil {
		fmt.Fprintln(os.Stderr, "goleak:", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

func newTestApp(t *testing.T) (*application, *testFakes) {
	t.Helper()
	fakes := newFakes()
	testAppOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		testShutdownCancel = cancel
		sharedApp = &application{
			config:      config{env: "testing"},
			logger:      slog.New(slog.NewJSONHandler(io.Discard, nil)),
			shutdownCtx: ctx,
		}
		sharedRouter = sharedApp.routes()
	})
	fakes.install(sharedApp)
	return sharedApp, fakes
}

func doRequest(t *testing.T, method, target, bearer string, body io.Reader) (*http.Response, []byte) {
	t.Helper()
	req := httptest.NewRequest(method, target, body)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rr := httptest.NewRecorder()
	sharedRouter.ServeHTTP(rr, req)
	res := rr.Result()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	_ = res.Body.Close()
	return res, b
}

func mustUnmarshal(t *testing.T, b []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("unmarshal response %q: %v", b, err)
	}
}

func mustErrorFields(t *testing.T, b []byte) map[string]string {
	t.Helper()
	var env struct {
		Error map[string]string `json:"error"`
	}
	mustUnmarshal(t, b, &env)
	return env.Error
}

func mustErrorMessage(t *testing.T, b []byte) string {
	t.Helper()
	var env struct {
		Error string `json:"error"`
	}
	mustUnmarshal(t, b, &env)
	return env.Error
}

func mustMessage(t *testing.T, b []byte) string {
	t.Helper()
	var env struct {
		Message string `json:"message"`
	}
	mustUnmarshal(t, b, &env)
	return env.Message
}
