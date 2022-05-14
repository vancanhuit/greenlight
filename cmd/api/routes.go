package main

import (
	"expvar"
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

	r.Use(app.metrics)
	r.Use(app.recoverPanic)
	r.Use(app.enableCORS)
	r.Use(app.rateLimit)
	r.Use(app.authenticate)

	r.Get("/v1/healthcheck", app.healtcheckHandler)
	r.With(app.requirePermissions("movies:read")).Get("/v1/movies", app.listMoviesHandler)
	r.With(app.requirePermissions("movies:read")).Get("/v1/movies/{id}", app.showMovieHandler)
	r.With(app.requirePermissions("movies:write")).Post("/v1/movies", app.createMovieHandler)
	r.With(app.requirePermissions("movies:write")).Patch("/v1/movies/{id}", app.updateMovieHandler)
	r.With(app.requirePermissions("movies:write")).Delete("/v1/movies/{id}", app.deleteMovieHandler)

	r.Post("/v1/users", app.registerUserHandler)
	r.Put("/v1/users/activated", app.activateUserHandler)

	r.Post("/v1/tokens/authentication", app.createAuthenticationTokenHandler)

	r.Get("/debug/vars", expvar.Handler().ServeHTTP)

	return r
}
