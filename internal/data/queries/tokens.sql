-- name: InsertToken :exec
INSERT INTO tokens (hash, user_id, expiry, scope)
VALUES ($1, $2, $3, $4);

-- name: DeleteAllTokensForUser :exec
DELETE FROM tokens WHERE scope = $1 AND user_id = $2;
