package data_test

import (
	"errors"
	"testing"

	"github.com/vancanhuit/greenlight/internal/data"
)

func TestMovieInsertGet(t *testing.T) {
	requireDB(t)
	truncate(t, "movies")

	models := data.NewModels(testPool)
	movie := &data.Movie{Title: "Casablanca", Year: 1942, Runtime: 102, Genres: []string{"drama"}}

	if err := models.Movies.Insert(movie); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if movie.ID == 0 {
		t.Fatal("expected generated ID")
	}

	got, err := models.Movies.Get(movie.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "Casablanca" || got.Year != 1942 || got.Runtime != 102 {
		t.Fatalf("unexpected movie: %+v", got)
	}
}

func TestMovieGetNotFound(t *testing.T) {
	requireDB(t)
	truncate(t, "movies")

	models := data.NewModels(testPool)
	_, err := models.Movies.Get(999)
	if !errors.Is(err, data.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound, got %v", err)
	}
}

func TestMovieUpdateEditConflict(t *testing.T) {
	requireDB(t)
	truncate(t, "movies")

	models := data.NewModels(testPool)
	movie := &data.Movie{Title: "Alien", Year: 1979, Runtime: 117, Genres: []string{"sci-fi"}}
	if err := models.Movies.Insert(movie); err != nil {
		t.Fatalf("insert: %v", err)
	}

	stale := *movie
	stale.Version = movie.Version + 1 // wrong version
	stale.Title = "Aliens"
	if err := models.Movies.Update(&stale); !errors.Is(err, data.ErrEditConflict) {
		t.Fatalf("expected ErrEditConflict, got %v", err)
	}
}

func TestMovieDelete(t *testing.T) {
	requireDB(t)
	truncate(t, "movies")

	models := data.NewModels(testPool)
	movie := &data.Movie{Title: "Heat", Year: 1995, Runtime: 170, Genres: []string{"crime"}}
	if err := models.Movies.Insert(movie); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := models.Movies.Delete(movie.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := models.Movies.Delete(movie.ID); !errors.Is(err, data.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound on second delete, got %v", err)
	}
}
