package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
)

// logging middleware to log each time there is a new request
type LoggingMiddleware struct {
	Logger *log.Logger
	Handler http.Handler
}

// log that a new request was made then call the next http handler
func (self LoggingMiddleware) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	self.Logger.Println("New Request")

	// TODO ideally we would wrap the response writer so we can read
	// the response before it gets sent back to the user
	// this would allow us to swap 500 level error descriptions for default 500 level errors
	// so that no sensitive info gets sent to the user
	// we could also log the descriptive 500 level error at this time

	self.Handler.ServeHTTP(writer, request)
}

func main() {
	// set the logger to log messages in UTC time
	log.SetFlags(log.LstdFlags | log.LUTC)

	log.Println("Server starting")

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

	// wrap the multiplexer in a middleware handler that logs when reqests are made
	serveHandler = LoggingMiddleware{
		Logger: log.Default(),
		Handler: serveHandler,
	}

	// create an http server for serving requests using the wrapped multiplexer we created
	var server = http.Server{
		Addr: fmt.Sprintf(":%d", serverPort),
		Handler: serveHandler,
	}

	// TODO run a routine watching for sigint so we can gracefully close the server

	log.Println("Server started successfully")

	// start the server
	var serverError error
	if shouldServeTls {
		serverError = server.ListenAndServeTLS(tlsCert, tlsKey)
	} else {
		serverError = server.ListenAndServe()
	}

	// serverError will always be a non nil value
	// check the reason that the server stopped
	// gracefully shutting down a server will return a http.ErrServerClosed error
	// we just want to log that the server has gracefully shut down if we see that
	// if we get any other error then we will log the error message
	if serverError == http.ErrServerClosed {
		log.Println("Server shutdown gracefully")
	} else {
		log.Printf("Server shutdown because an error occured: %s\n", serverError)
	}
}
