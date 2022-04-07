package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
)

func main() {
	// variables that will be set to values supplied by the user via the command line
	var serverPort int
	var shouldServeTls bool

	flag.IntVar(&serverPort, "p", 80, "The TCP port for the server to listen on")
	flag.BoolVar(&shouldServeTls, "t", false, "Handle requests using TLS encryption")

	// parse the command line args for flag values
	flag.Parse()

	var tlsCert string
	var tlsKey string

	// set the default port to 443 if tls was requested and the server port was not explicitly set
	if shouldServeTls {
		var portExplictlySet bool

		// check if the port flag was explicitly set
		// by iterating over all of the flag values that were set
		flag.Visit(func(f *flag.Flag) {
			if f.Name == "p" {
				portExplictlySet = true
			}
		})

		if portExplictlySet == false {
			serverPort = 443
		}

		// get the cert and key values from env variables
		tlsCert = os.Getenv("AUDIT_LOG_TLS_CERT")
		tlsKey = os.Getenv("AUDIT_LOG_TLS_KEY")
	}

	// create a new http multiplexer for handling http requests
	var mux = http.NewServeMux()

	// the http handler that will be used to serve http requests
	var serveHandler http.Handler = mux

	// create an http server for serving requests using the wrapped multiplexer we created
	var server = http.Server{
		Addr: fmt.Sprintf(":%d", serverPort),
		Handler: serveHandler,
	}

	// start the server
	var serverError error
	if shouldServeTls {
		serverError = server.ListenAndServeTLS(tlsCert, tlsKey)
	} else {
		serverError = server.ListenAndServe()
	}

	_ = serverError
}
