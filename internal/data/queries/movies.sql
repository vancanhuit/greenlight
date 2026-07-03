-- name: InsertMovie :one
INSERT INTO movies (title, year, runtime, genres)
VALUES ($1, $2, $3, $4)
RETURNING id, created_at, version;

-- name: GetMovie :one
SELECT id, created_at, title, year, runtime, genres, version
FROM movies
WHERE id = $1;

-- name: UpdateMovie :one
UPDATE movies
SET title = $1, year = $2, runtime = $3, genres = $4, version = version + 1
WHERE id = $5 AND version = $6
RETURNING version;

-- name: DeleteMovie :execrows
DELETE FROM movies WHERE id = $1;
