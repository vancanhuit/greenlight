package data

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	db "github.com/vancanhuit/greenlight/internal/data/sqlc"
)

type Models struct {
	Movies      MovieModel
	Users       UserModel
	Tokens      TokenModel
	Permissions PermissionModel
}

var (
	ErrRecordNotFound = errors.New("record not found")
	ErrEditConflict   = errors.New("edit conflict")
	ErrDuplicateEmail = errors.New("duplicate email")
)

func NewModels(pool *pgxpool.Pool) Models {
	q := db.New(pool)
	return Models{
		Movies:      MovieModel{q: q, pool: pool},
		Users:       UserModel{q: q, pool: pool},
		Tokens:      TokenModel{q: q, pool: pool},
		Permissions: PermissionModel{q: q, pool: pool},
	}
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == constraint
	}
	return false
}
