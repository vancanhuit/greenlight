package data_test

import (
	"errors"
	"slices"
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

func TestMovieUpdate(t *testing.T) {
	requireDB(t)
	truncate(t, "movies")

	models := data.NewModels(testPool)
	movie := &data.Movie{Title: "Blade Runner", Year: 1982, Runtime: 117, Genres: []string{"sci-fi"}}
	if err := models.Movies.Insert(movie); err != nil {
		t.Fatalf("insert: %v", err)
	}
	originalVersion := movie.Version

	movie.Title = "Blade Runner: The Final Cut"
	movie.Runtime = 118
	if err := models.Movies.Update(movie); err != nil {
		t.Fatalf("update: %v", err)
	}
	if movie.Version != originalVersion+1 {
		t.Fatalf("expected version %d, got %d", originalVersion+1, movie.Version)
	}

	got, err := models.Movies.Get(movie.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "Blade Runner: The Final Cut" || got.Runtime != 118 {
		t.Fatalf("unexpected movie after update: %+v", got)
	}
	if got.Version != movie.Version {
		t.Fatalf("stored version %d, want %d", got.Version, movie.Version)
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

// seedMovies inserts a fixed set of movies used by the GetAll tests and returns
// them keyed by title for easy ID lookups.
func seedMovies(t *testing.T, models data.Models) map[string]*data.Movie {
	t.Helper()
	seed := []*data.Movie{
		{Title: "The Matrix", Year: 1999, Runtime: 136, Genres: []string{"sci-fi", "action"}},
		{Title: "Alien", Year: 1979, Runtime: 117, Genres: []string{"sci-fi", "horror"}},
		{Title: "Casablanca", Year: 1942, Runtime: 102, Genres: []string{"drama", "romance"}},
		{Title: "Heat", Year: 1995, Runtime: 170, Genres: []string{"crime", "drama"}},
	}
	byTitle := make(map[string]*data.Movie, len(seed))
	for _, m := range seed {
		if err := models.Movies.Insert(m); err != nil {
			t.Fatalf("seed insert %q: %v", m.Title, err)
		}
		byTitle[m.Title] = m
	}
	return byTitle
}

func newFilters(sort string) data.Filters {
	return data.Filters{
		Page:         1,
		PageSize:     20,
		Sort:         sort,
		SortSafelist: []string{"id", "title", "year", "runtime", "-id", "-title", "-year", "-runtime"},
	}
}

func titles(movies []*data.Movie) []string {
	out := make([]string, len(movies))
	for i, m := range movies {
		out[i] = m.Title
	}
	return out
}

func TestMovieGetAllSorting(t *testing.T) {
	requireDB(t)
	truncate(t, "movies")

	models := data.NewModels(testPool)
	seedMovies(t, models)

	tests := []struct {
		name string
		sort string
		want []string
	}{
		{"id ASC", "id", []string{"The Matrix", "Alien", "Casablanca", "Heat"}},
		{"id DESC", "-id", []string{"Heat", "Casablanca", "Alien", "The Matrix"}},
		{"title ASC", "title", []string{"Alien", "Casablanca", "Heat", "The Matrix"}},
		{"title DESC", "-title", []string{"The Matrix", "Heat", "Casablanca", "Alien"}},
		{"year ASC", "year", []string{"Casablanca", "Alien", "Heat", "The Matrix"}},
		{"year DESC", "-year", []string{"The Matrix", "Heat", "Alien", "Casablanca"}},
		{"runtime ASC", "runtime", []string{"Casablanca", "Alien", "The Matrix", "Heat"}},
		{"runtime DESC", "-runtime", []string{"Heat", "The Matrix", "Alien", "Casablanca"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, metadata, err := models.Movies.GetAll("", []string{}, newFilters(tc.sort))
			if err != nil {
				t.Fatalf("GetAll: %v", err)
			}
			if names := titles(got); !slices.Equal(names, tc.want) {
				t.Fatalf("sort %q: got order %v, want %v", tc.sort, names, tc.want)
			}
			if metadata.TotalRecords != 4 {
				t.Fatalf("sort %q: got TotalRecords %d, want 4", tc.sort, metadata.TotalRecords)
			}
		})
	}
}

func TestMovieGetAllTitleFilter(t *testing.T) {
	requireDB(t)
	truncate(t, "movies")

	models := data.NewModels(testPool)
	seedMovies(t, models)

	got, metadata, err := models.Movies.GetAll("matrix", []string{}, newFilters("id"))
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if names := titles(got); !slices.Equal(names, []string{"The Matrix"}) {
		t.Fatalf("title filter: got %v, want [The Matrix]", names)
	}
	if metadata.TotalRecords != 1 {
		t.Fatalf("title filter: got TotalRecords %d, want 1", metadata.TotalRecords)
	}
}

func TestMovieGetAllGenresFilter(t *testing.T) {
	requireDB(t)
	truncate(t, "movies")

	models := data.NewModels(testPool)
	seedMovies(t, models)

	got, metadata, err := models.Movies.GetAll("", []string{"sci-fi"}, newFilters("title"))
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if names := titles(got); !slices.Equal(names, []string{"Alien", "The Matrix"}) {
		t.Fatalf("genres filter: got %v, want [Alien The Matrix]", names)
	}
	if metadata.TotalRecords != 2 {
		t.Fatalf("genres filter: got TotalRecords %d, want 2", metadata.TotalRecords)
	}
}

func TestMovieGetAllPaginationMetadata(t *testing.T) {
	requireDB(t)
	truncate(t, "movies")

	models := data.NewModels(testPool)
	seedMovies(t, models)

	filters := data.Filters{
		Page:         2,
		PageSize:     2,
		Sort:         "id",
		SortSafelist: []string{"id"},
	}
	got, metadata, err := models.Movies.GetAll("", []string{}, filters)
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if names := titles(got); !slices.Equal(names, []string{"Casablanca", "Heat"}) {
		t.Fatalf("pagination: got %v, want [Casablanca Heat]", names)
	}
	want := data.Metadata{
		CurrentPage:  2,
		PageSize:     2,
		FirstPage:    1,
		LastPage:     2,
		TotalRecords: 4,
	}
	if metadata != want {
		t.Fatalf("pagination metadata: got %+v, want %+v", metadata, want)
	}
}

func TestMovieGetAllEmpty(t *testing.T) {
	requireDB(t)
	truncate(t, "movies")

	models := data.NewModels(testPool)
	got, metadata, err := models.Movies.GetAll("", []string{}, newFilters("id"))
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no movies, got %v", titles(got))
	}
	if metadata != (data.Metadata{}) {
		t.Fatalf("expected empty metadata, got %+v", metadata)
	}
}
