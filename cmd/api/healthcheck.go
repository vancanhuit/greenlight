package main

import (
	"fmt"
	"net/http"
)

func (app *application) healtcheckHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "status: available\n")
	fmt.Fprintf(w, "environment: %s\n", app.config.env)
	fmt.Fprintf(w, "version %s\n", version)
}
