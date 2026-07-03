-- name: InsertUser :one
INSERT INTO users (name, email, password_hash, activated)
VALUES ($1, $2, $3, $4)
RETURNING id, created_at, version;

-- name: GetUserByEmail :one
SELECT id, created_at, name, email, password_hash, activated, version
FROM users
WHERE email = $1;

-- name: UpdateUser :one
UPDATE users
SET name = $1, email = $2, password_hash = $3, activated = $4, version = version + 1
WHERE id = $5 AND version = $6
RETURNING version;

-- name: GetUserForToken :one
SELECT users.id, users.created_at, users.name, users.email, users.password_hash, users.activated, users.version
FROM users
INNER JOIN tokens ON users.id = tokens.user_id
WHERE tokens.hash = $1 AND tokens.scope = $2 AND tokens.expiry > $3;
