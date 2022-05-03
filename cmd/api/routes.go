package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (app *application) routes() http.Handler {
	r := chi.NewRouter()

	r.Get("/v1/healthcheck", app.healtcheckHandler)
	r.Get("/v1/movies/{id}", app.showMovieHandler)
	r.Post("/v1/movies", app.createMovieHandler)

	return r
}