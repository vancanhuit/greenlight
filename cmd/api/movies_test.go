package main

import (
	"bytes"
	"net/http"
	"reflect"
	"testing"

	"github.com/vancanhuit/greenlight/internal/data"
)

func TestCreateMovieHandler(t *testing.T) {
	_, fakes := newTestApp(t)
	_, token := fakes.seedAuthedUser(t, "movies:write")

	reqBody := `{"title":"Black Panther","year":2018,"runtime":"134 mins","genres":["action","adventure"]}`
	res, resBody := doRequest(t, http.MethodPost, "/v1/movies", token, bytes.NewBufferString(reqBody))

	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status: got %d want %d (body: %s)", res.StatusCode, http.StatusCreated, resBody)
	}
	if loc := res.Header.Get("Location"); loc != "/v1/movies/1" {
		t.Errorf("Location: got %q want %q", loc, "/v1/movies/1")
	}

	var env struct {
		Movie struct {
			ID     int64    `json:"id"`
			Title  string   `json:"title"`
			Year   int32    `json:"year"`
			Genres []string `json:"genres"`
		} `json:"movie"`
	}
	mustUnmarshal(t, resBody, &env)
	if env.Movie.ID != 1 || env.Movie.Title != "Black Panther" || env.Movie.Year != 2018 {
		t.Errorf("unexpected movie envelope: %+v", env.Movie)
	}
	if _, ok := fakes.movies.movies[1]; !ok {
		t.Error("expected movie to be persisted in the store")
	}
}

func TestCreateMovieHandlerValidationFailure(t *testing.T) {
	_, fakes := newTestApp(t)
	_, token := fakes.seedAuthedUser(t, "movies:write")

	res, resBody := doRequest(t, http.MethodPost, "/v1/movies", token, bytes.NewBufferString(`{}`))

	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status: got %d want %d (body: %s)", res.StatusCode, http.StatusUnprocessableEntity, resBody)
	}

	fieldErrors := mustErrorFields(t, resBody)
	for _, field := range []string{"title", "year", "runtime", "genres"} {
		if _, ok := fieldErrors[field]; !ok {
			t.Errorf("expected validation error for field %q, got %v", field, fieldErrors)
		}
	}
	if len(fakes.movies.movies) != 0 {
		t.Error("expected no movie to be persisted on validation failure")
	}
}

func TestShowMovieHandler(t *testing.T) {
	_, fakes := newTestApp(t)
	_, token := fakes.seedAuthedUser(t, "movies:read")
	fakes.movies.seed(&data.Movie{ID: 1, Title: "Moana", Year: 2016, Runtime: 107, Genres: []string{"animation"}, Version: 1})

	res, resBody := doRequest(t, http.MethodGet, "/v1/movies/1", token, nil)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d (body: %s)", res.StatusCode, http.StatusOK, resBody)
	}
	var env struct {
		Movie struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"movie"`
	}
	mustUnmarshal(t, resBody, &env)
	if env.Movie.ID != 1 || env.Movie.Title != "Moana" {
		t.Errorf("unexpected movie envelope: %+v", env.Movie)
	}
}

func TestShowMovieHandlerNotFound(t *testing.T) {
	_, fakes := newTestApp(t)
	_, token := fakes.seedAuthedUser(t, "movies:read")

	res, resBody := doRequest(t, http.MethodGet, "/v1/movies/999", token, nil)

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status: got %d want %d (body: %s)", res.StatusCode, http.StatusNotFound, resBody)
	}
	if msg := mustErrorMessage(t, resBody); msg != "the requested resource could not be found" {
		t.Errorf("unexpected error message: %q", msg)
	}
}

func TestShowMovieHandlerBadID(t *testing.T) {
	_, fakes := newTestApp(t)
	_, token := fakes.seedAuthedUser(t, "movies:read")

	res, resBody := doRequest(t, http.MethodGet, "/v1/movies/abc", token, nil)

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status: got %d want %d (body: %s)", res.StatusCode, http.StatusNotFound, resBody)
	}
}

func TestListMoviesHandler(t *testing.T) {
	_, fakes := newTestApp(t)
	_, token := fakes.seedAuthedUser(t, "movies:read")
	fakes.movies.listMovies = []*data.Movie{
		{ID: 1, Title: "M1"},
		{ID: 2, Title: "M2"},
	}
	fakes.movies.listMetadata = data.Metadata{CurrentPage: 2, PageSize: 5, TotalRecords: 12}

	res, resBody := doRequest(t, http.MethodGet, "/v1/movies?title=black&genres=action,drama&sort=-year&page=2&page_size=5", token, nil)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d (body: %s)", res.StatusCode, http.StatusOK, resBody)
	}

	if fakes.movies.gotTitle != "black" {
		t.Errorf("title forwarded to store: got %q want %q", fakes.movies.gotTitle, "black")
	}
	if !reflect.DeepEqual(fakes.movies.gotGenres, []string{"action", "drama"}) {
		t.Errorf("genres forwarded to store: got %v want %v", fakes.movies.gotGenres, []string{"action", "drama"})
	}
	if fakes.movies.gotFilters.Sort != "-year" || fakes.movies.gotFilters.Page != 2 || fakes.movies.gotFilters.PageSize != 5 {
		t.Errorf("filters forwarded to store: got %+v", fakes.movies.gotFilters)
	}

	var env struct {
		Movies []struct {
			ID int64 `json:"id"`
		} `json:"movies"`
		Metadata data.Metadata `json:"metadata"`
	}
	mustUnmarshal(t, resBody, &env)
	if len(env.Movies) != 2 {
		t.Errorf("movies count: got %d want 2", len(env.Movies))
	}
	if env.Metadata.CurrentPage != 2 || env.Metadata.TotalRecords != 12 {
		t.Errorf("unexpected metadata: %+v", env.Metadata)
	}
}

func TestUpdateMovieHandler(t *testing.T) {
	_, fakes := newTestApp(t)
	_, token := fakes.seedAuthedUser(t, "movies:write")
	fakes.movies.seed(&data.Movie{ID: 1, Title: "Old", Year: 2000, Runtime: 100, Genres: []string{"drama"}, Version: 1})

	res, resBody := doRequest(t, http.MethodPatch, "/v1/movies/1", token, bytes.NewBufferString(`{"title":"New Title"}`))

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d (body: %s)", res.StatusCode, http.StatusOK, resBody)
	}
	var env struct {
		Movie struct {
			Title   string `json:"title"`
			Version int32  `json:"version"`
		} `json:"movie"`
	}
	mustUnmarshal(t, resBody, &env)
	if env.Movie.Title != "New Title" {
		t.Errorf("title: got %q want %q", env.Movie.Title, "New Title")
	}
	if env.Movie.Version != 2 {
		t.Errorf("version should be bumped: got %d want 2", env.Movie.Version)
	}
}

func TestUpdateMovieHandlerNotFound(t *testing.T) {
	_, fakes := newTestApp(t)
	_, token := fakes.seedAuthedUser(t, "movies:write")

	res, resBody := doRequest(t, http.MethodPatch, "/v1/movies/999", token, bytes.NewBufferString(`{"title":"New Title"}`))

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status: got %d want %d (body: %s)", res.StatusCode, http.StatusNotFound, resBody)
	}
}

func TestUpdateMovieHandlerEditConflict(t *testing.T) {
	_, fakes := newTestApp(t)
	_, token := fakes.seedAuthedUser(t, "movies:write")
	fakes.movies.seed(&data.Movie{ID: 1, Title: "Old", Year: 2000, Runtime: 100, Genres: []string{"drama"}, Version: 1})
	fakes.movies.updateErr = data.ErrEditConflict

	res, resBody := doRequest(t, http.MethodPatch, "/v1/movies/1", token, bytes.NewBufferString(`{"title":"New Title"}`))

	if res.StatusCode != http.StatusConflict {
		t.Fatalf("status: got %d want %d (body: %s)", res.StatusCode, http.StatusConflict, resBody)
	}
	if msg := mustErrorMessage(t, resBody); msg != "unable to update the record due to an edit conflict, please try again" {
		t.Errorf("unexpected error message: %q", msg)
	}
}

func TestDeleteMovieHandler(t *testing.T) {
	_, fakes := newTestApp(t)
	_, token := fakes.seedAuthedUser(t, "movies:write")
	fakes.movies.seed(&data.Movie{ID: 1, Title: "Old", Year: 2000, Runtime: 100, Genres: []string{"drama"}, Version: 1})

	res, resBody := doRequest(t, http.MethodDelete, "/v1/movies/1", token, nil)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d (body: %s)", res.StatusCode, http.StatusOK, resBody)
	}
	if msg := mustMessage(t, resBody); msg != "movie sucessfully deleted" {
		t.Errorf("unexpected message: %q", msg)
	}
	if _, ok := fakes.movies.movies[1]; ok {
		t.Error("expected movie to be removed from the store")
	}
}

func TestDeleteMovieHandlerNotFound(t *testing.T) {
	_, fakes := newTestApp(t)
	_, token := fakes.seedAuthedUser(t, "movies:write")

	res, resBody := doRequest(t, http.MethodDelete, "/v1/movies/999", token, nil)

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status: got %d want %d (body: %s)", res.StatusCode, http.StatusNotFound, resBody)
	}
}

func TestMovieRouteRequiresPermission(t *testing.T) {
	_, fakes := newTestApp(t)
	// Authenticated & activated, but without the movies:write permission.
	_, token := fakes.seedAuthedUser(t, "movies:read")

	reqBody := `{"title":"Black Panther","year":2018,"runtime":"134 mins","genres":["action"]}`
	res, resBody := doRequest(t, http.MethodPost, "/v1/movies", token, bytes.NewBufferString(reqBody))

	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status: got %d want %d (body: %s)", res.StatusCode, http.StatusForbidden, resBody)
	}
}
