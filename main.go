package main

import (
	"net/http"
)

func main() {
	// create a new http multiplexer for handling http requests
	var mux = http.NewServeMux()

	// the http handler that will be used to serve http requests
	var serveHandler http.Handler = mux

	// create an http server for serving requests using the wrapped multiplexer we created
	var server = http.Server{
		Handler: serveHandler,
	}

	var serverError = server.ListenAndServe()
	_ = serverError
}
