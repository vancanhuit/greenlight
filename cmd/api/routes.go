package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (app *application) routes() http.Handler {
	r := chi.NewRouter()

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		app.notFoundResponse(w, r)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		app.methodNotAllowedResponse(w, r)
	})

	r.Get("/v1/healthcheck", app.healtcheckHandler)
	r.Get("/v1/movies", app.listMoviesHandler)
	r.Get("/v1/movies/{id}", app.showMovieHandler)
	r.Post("/v1/movies", app.createMovieHandler)
	r.Patch("/v1/movies/{id}", app.updateMovieHandler)
	r.Delete("/v1/movies/{id}", app.deleteMovieHandler)

	return r
}
