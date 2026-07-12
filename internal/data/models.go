package data

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	db "github.com/vancanhuit/greenlight/internal/data/sqlc"
)

type Models struct {
	Movies      MovieModel
	Users       UserModel
	Tokens      TokenModel
	Permissions PermissionModel

	pool *pgxpool.Pool
	q    *db.Queries
}

var (
	ErrRecordNotFound = errors.New("record not found")
	ErrEditConflict   = errors.New("edit conflict")
	ErrDuplicateEmail = errors.New("duplicate email")
)

func NewModels(pool *pgxpool.Pool) Models {
	q := db.New(pool)
	return Models{
		Movies:      MovieModel{q: q},
		Users:       UserModel{q: q},
		Tokens:      TokenModel{q: q},
		Permissions: PermissionModel{q: q},
		pool:        pool,
		q:           q,
	}
}

// RegisterUser inserts a user, grants the given permissions, and creates an
// activation token atomically. Either every write commits or none do, so a
// failure never leaves an orphaned user without permissions or a token.
func (m Models) RegisterUser(user *User, permissions []string, tokenTTL time.Duration, tokenScope string) (*Token, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op once committed

	qtx := m.q.WithTx(tx)

	row, err := qtx.InsertUser(ctx, db.InsertUserParams{
		Name:         user.Name,
		Email:        user.Email,
		PasswordHash: user.Password.hash,
		Activated:    user.Activated,
	})
	if err != nil {
		if isUniqueViolation(err, "users_email_key") {
			return nil, ErrDuplicateEmail
		}
		return nil, err
	}
	user.ID = row.ID
	user.CreatedAt = row.CreatedAt
	user.Version = int(row.Version)

	if len(permissions) > 0 {
		if err := qtx.AddPermissionsForUser(ctx, db.AddPermissionsForUserParams{
			UserID:  user.ID,
			Column2: permissions,
		}); err != nil {
			return nil, err
		}
	}

	token, err := generateToken(user.ID, tokenTTL, tokenScope)
	if err != nil {
		return nil, err
	}
	if err := qtx.InsertToken(ctx, db.InsertTokenParams{
		Hash:   token.Hash,
		UserID: token.UserID,
		Expiry: token.Expiry,
		Scope:  token.Scope,
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return token, nil
}

// ActivateUser persists the user's updated state and deletes their tokens for
// the given scope atomically, closing the window where an activated account
// still has usable activation tokens.
func (m Models) ActivateUser(user *User, tokenScope string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op once committed

	qtx := m.q.WithTx(tx)

	version, err := qtx.UpdateUser(ctx, db.UpdateUserParams{
		Name:         user.Name,
		Email:        user.Email,
		PasswordHash: user.Password.hash,
		Activated:    user.Activated,
		ID:           user.ID,
		Version:      int32(user.Version),
	})
	if err != nil {
		switch {
		case isUniqueViolation(err, "users_email_key"):
			return ErrDuplicateEmail
		case errors.Is(err, pgx.ErrNoRows):
			return ErrEditConflict
		default:
			return err
		}
	}
	user.Version = int(version)

	if err := qtx.DeleteAllTokensForUser(ctx, db.DeleteAllTokensForUserParams{
		Scope:  tokenScope,
		UserID: user.ID,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == constraint
	}
	return false
}
