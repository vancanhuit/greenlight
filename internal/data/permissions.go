package data

import (
	"context"
	"time"

	db "github.com/vancanhuit/greenlight/internal/data/sqlc"
)

type Permissions []string

func (p Permissions) Include(code string) bool {
	for i := range p {
		if code == p[i] {
			return true
		}
	}
	return false
}

type PermissionModel struct {
	q *db.Queries
}

func (m PermissionModel) GetAllForUser(userID int64) (Permissions, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	codes, err := m.q.GetPermissionsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return Permissions(codes), nil
}

func (m PermissionModel) AddForUser(userID int64, codes ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return m.q.AddPermissionsForUser(ctx, db.AddPermissionsForUserParams{
		UserID:  userID,
		Column2: codes,
	})
}
