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

-- name: ListMovies :many
SELECT count(*) OVER() AS total, id, created_at, title, year, runtime, genres, version
FROM movies
WHERE (to_tsvector('simple', title) @@ plainto_tsquery('simple', @title) OR @title = '')
  AND (genres @> @genres::text[] OR @genres::text[] = '{}')
ORDER BY
  CASE WHEN @sort_column::text = 'id'      AND @sort_direction::text = 'ASC'  THEN id      END ASC,
  CASE WHEN @sort_column::text = 'id'      AND @sort_direction::text = 'DESC' THEN id      END DESC,
  CASE WHEN @sort_column::text = 'title'   AND @sort_direction::text = 'ASC'  THEN title   END ASC,
  CASE WHEN @sort_column::text = 'title'   AND @sort_direction::text = 'DESC' THEN title   END DESC,
  CASE WHEN @sort_column::text = 'year'    AND @sort_direction::text = 'ASC'  THEN year    END ASC,
  CASE WHEN @sort_column::text = 'year'    AND @sort_direction::text = 'DESC' THEN year    END DESC,
  CASE WHEN @sort_column::text = 'runtime' AND @sort_direction::text = 'ASC'  THEN runtime END ASC,
  CASE WHEN @sort_column::text = 'runtime' AND @sort_direction::text = 'DESC' THEN runtime END DESC,
  id
LIMIT @page_limit OFFSET @page_offset;
