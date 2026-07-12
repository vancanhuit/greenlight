package data

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	db "github.com/vancanhuit/greenlight/internal/data/sqlc"
	"github.com/vancanhuit/greenlight/internal/validator"
	"golang.org/x/crypto/bcrypt"
)

type password struct {
	plaintext *string
	hash      []byte
}

type User struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Password  password  `json:"-"`
	Activated bool      `json:"activated"`
	Version   int       `json:"-"`
}

type UserModel struct {
	q *db.Queries
}

var AnonymousUser = &User{}

func (u *User) IsAnonymous() bool {
	return u == AnonymousUser
}

func (p *password) Set(plaintextPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintextPassword), 12)
	if err != nil {
		return err
	}

	p.plaintext = &plaintextPassword
	p.hash = hash

	return nil
}

func (p *password) Matches(plaintextPassword string) (bool, error) {
	err := bcrypt.CompareHashAndPassword(p.hash, []byte(plaintextPassword))
	if err != nil {
		switch {
		case errors.Is(err, bcrypt.ErrMismatchedHashAndPassword):
			return false, nil
		default:
			return false, err
		}
	}

	return true, nil
}

func ValidateEmail(v *validator.Validator, email string) {
	v.Check(email != "", "email", "must be provided")
	v.Check(validator.Matches(email, validator.EmailRX), "email", "must be a valid email address")
}

func ValidatePasswordPlaintext(v *validator.Validator, password string) {
	v.Check(password != "", "password", "must be provided")
	v.Check(len(password) >= 8, "password", "must be at least 8 characters long")
	v.Check(len(password) <= 72, "password", "must not be more than 72 bytes long")
}

func ValidateUser(v *validator.Validator, user *User) {
	v.Check(user.Name != "", "name", "must be provided")
	v.Check(len(user.Name) <= 500, "name", "must not be more than 500 bytes long")

	ValidateEmail(v, user.Email)
	if user.Password.plaintext != nil {
		ValidatePasswordPlaintext(v, *user.Password.plaintext)
	}

	if user.Password.hash == nil {
		panic("missing password hash for user")
	}
}

func (m UserModel) Insert(user *User) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	row, err := m.q.InsertUser(ctx, db.InsertUserParams{
		Name:         user.Name,
		Email:        user.Email,
		PasswordHash: user.Password.hash,
		Activated:    user.Activated,
	})
	if err != nil {
		if isUniqueViolation(err, "users_email_key") {
			return ErrDuplicateEmail
		}
		return err
	}
	user.ID = row.ID
	user.CreatedAt = row.CreatedAt
	user.Version = int(row.Version)
	return nil
}

func (m UserModel) GetByEmail(email string) (*User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	row, err := m.q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, err
	}
	return userFromRow(row.ID, row.CreatedAt, row.Name, row.Email, row.PasswordHash, row.Activated, row.Version), nil
}

func (m UserModel) GetForToken(tokenScope string, tokenPlaintext string) (*User, error) {
	tokenHash := sha256.Sum256([]byte(tokenPlaintext))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	row, err := m.q.GetUserForToken(ctx, db.GetUserForTokenParams{
		Hash:   tokenHash[:],
		Scope:  tokenScope,
		Expiry: time.Now(),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, err
	}
	return userFromRow(row.ID, row.CreatedAt, row.Name, row.Email, row.PasswordHash, row.Activated, row.Version), nil
}

func userFromRow(id int64, createdAt time.Time, name, email string, passwordHash []byte, activated bool, version int32) *User {
	u := &User{
		ID:        id,
		CreatedAt: createdAt,
		Name:      name,
		Email:     email,
		Activated: activated,
		Version:   int(version),
	}
	u.Password.hash = passwordHash
	return u
}
