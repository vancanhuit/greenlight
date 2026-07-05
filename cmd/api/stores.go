package main

import (
	"time"

	"github.com/vancanhuit/greenlight/internal/data"
)

// MovieStore is the seam the movie handlers depend on.
type MovieStore interface {
	Insert(movie *data.Movie) error
	Get(id int64) (*data.Movie, error)
	GetAll(title string, genres []string, filters data.Filters) ([]*data.Movie, data.Metadata, error)
	Update(movie *data.Movie) error
	Delete(id int64) error
}

// UserStore is the seam the user and token handlers depend on.
type UserStore interface {
	Insert(user *data.User) error
	GetByEmail(email string) (*data.User, error)
	Update(user *data.User) error
	GetForToken(tokenScope, tokenPlaintext string) (*data.User, error)
}

// TokenStore is the seam the token and user handlers depend on.
type TokenStore interface {
	Insert(token *data.Token) error
	DeleteAllForUser(scope string, userID int64) error
	New(userID int64, ttl time.Duration, scope string) (*data.Token, error)
}

// PermissionStore is the seam the permission-aware handlers depend on.
type PermissionStore interface {
	GetAllForUser(userID int64) (data.Permissions, error)
	AddForUser(userID int64, codes ...string) error
}

// Emailer is the seam the user handlers depend on for sending mail.
type Emailer interface {
	Send(recipient, templateFile string, data any) error
}
