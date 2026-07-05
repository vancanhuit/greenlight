package data

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	db "github.com/vancanhuit/greenlight/internal/data/sqlc"
	"github.com/vancanhuit/greenlight/internal/validator"
)

type Movie struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"-"`
	Title     string    `json:"title"`
	Year      int32     `json:"year,omitempty"`
	Runtime   Runtime   `json:"runtime,omitempty"`
	Genres    []string  `json:"genres,omitempty"`
	Version   int32     `json:"version"`
}

type MovieModel struct {
	q *db.Queries
}

func ValidateMovie(v *validator.Validator, movie *Movie) {
	v.Check(movie.Title != "", "title", "must be provided")
	v.Check(len(movie.Title) <= 500, "title", "must not be more than 500 bytes long")

	v.Check(movie.Year != 0, "year", "must be provided")
	v.Check(movie.Year >= 1888, "year", "must be greater than or equal 1888")
	v.Check(movie.Year <= int32(time.Now().Year()), "year", "must not be in the future")

	v.Check(movie.Runtime != 0, "runtime", "must be provided")
	v.Check(movie.Runtime > 0, "runtime", "must be a positive integer")

	v.Check(movie.Genres != nil, "genres", "must be provided")
	v.Check(len(movie.Genres) >= 1, "genres", "must contain at least 1 genre")
	v.Check(len(movie.Genres) <= 5, "genres", "must not contain more than 5 genres")
	v.Check(validator.Unique(movie.Genres), "genres", "must not contain duplicate values")
}

func (m MovieModel) Insert(movie *Movie) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	row, err := m.q.InsertMovie(ctx, db.InsertMovieParams{
		Title:   movie.Title,
		Year:    movie.Year,
		Runtime: int32(movie.Runtime),
		Genres:  movie.Genres,
	})
	if err != nil {
		return err
	}
	movie.ID = row.ID
	movie.CreatedAt = row.CreatedAt
	movie.Version = row.Version
	return nil
}

func (m MovieModel) Get(id int64) (*Movie, error) {
	if id < 1 {
		return nil, ErrRecordNotFound
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	row, err := m.q.GetMovie(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, err
	}

	return &Movie{
		ID:        row.ID,
		CreatedAt: row.CreatedAt,
		Title:     row.Title,
		Year:      row.Year,
		Runtime:   Runtime(row.Runtime),
		Genres:    row.Genres,
		Version:   row.Version,
	}, nil
}

func (m MovieModel) GetAll(title string, genres []string, filters Filters) ([]*Movie, Metadata, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	rows, err := m.q.ListMovies(ctx, db.ListMoviesParams{
		Title:         title,
		Genres:        genres,
		SortColumn:    filters.sortColumn(),
		SortDirection: filters.sortDirection(),
		PageLimit:     int32(filters.limit()),
		PageOffset:    int32(filters.offset()),
	})
	if err != nil {
		return nil, Metadata{}, err
	}

	movies := []*Movie{}
	totalRecords := 0
	for _, row := range rows {
		totalRecords = int(row.Total)
		movies = append(movies, &Movie{
			ID:        row.ID,
			CreatedAt: row.CreatedAt,
			Title:     row.Title,
			Year:      row.Year,
			Runtime:   Runtime(row.Runtime),
			Genres:    row.Genres,
			Version:   row.Version,
		})
	}

	metadata := calculateMetadata(totalRecords, filters.Page, filters.PageSize)
	return movies, metadata, nil
}

func (m MovieModel) Update(movie *Movie) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	version, err := m.q.UpdateMovie(ctx, db.UpdateMovieParams{
		Title:   movie.Title,
		Year:    movie.Year,
		Runtime: int32(movie.Runtime),
		Genres:  movie.Genres,
		ID:      movie.ID,
		Version: movie.Version,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrEditConflict
		}
		return err
	}
	movie.Version = version
	return nil
}

func (m MovieModel) Delete(id int64) error {
	if id < 1 {
		return ErrRecordNotFound
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	rows, err := m.q.DeleteMovie(ctx, id)
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrRecordNotFound
	}
	return nil
}
