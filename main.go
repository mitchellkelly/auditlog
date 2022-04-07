package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"

	"github.com/qri-io/jsonschema"
)

type HttpError struct {
	Code        int    `json:"-"`
	Description string `json:"description"`
}

func (self HttpError) Error() string {
	return self.Description
}

func DefaultHttpError(statusCode int) HttpError {
	return HttpError{
		Code:        statusCode,
		Description: http.StatusText(statusCode),
	}
}

func WriteJsonResponse(writer http.ResponseWriter, v interface{}) {
	var statusCode int
	var responseBytes []byte

	if v != nil {
		// check the type of v to determine if it is an error
		var e, ok = v.(error)

		if ok {
			// narrow the error down further to determine if it is an HttpError
			httpErr, ok := e.(HttpError)
			// if the error was not an http error then we have an internal server error
			if !ok {
				v = e.Error()

				statusCode = 500
			} else {
				statusCode = httpErr.Code
			}
		}

		var err error
		// marshal the response object into json so we can send it to the user
		responseBytes, err = json.Marshal(v)

		// if marshaling the json was successful then we will return the user provided status code if one was set
		// or a 200 if nothing was set by the user
		// if an error occured while marshaling the object to json then we will return a plain 500 error
		if err == nil {
			if statusCode == 0 {
				statusCode = http.StatusOK
			}
		} else {
			statusCode = http.StatusInternalServerError
			responseBytes = []byte(fmt.Sprintf(`{"description": "%s"}`, http.StatusText(statusCode)))
		}
	} else {
		// if v is nil then the user does not want to write anything
		// just return a 204 and an empty json body
		statusCode = http.StatusNoContent
		responseBytes = []byte("{}")
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(responseBytes)))
	writer.WriteHeader(statusCode)
	writer.Write(responseBytes)
}

// TODO the custom http mux code (middlewares and routers) could be replaced
// with a more sophisticated mux package (i prefer github.com/gorilla/mux)
// the custom code is used here so that this service can mostly use features
// already available in Go

// http handler that authenticates a request and calls another http handler
// if authentication is successful
type AuthenticationMiddleware struct {
	// token to use when authenticating requests
	Token string
	// http handler to call if authentication succeeds
	Handler http.Handler
}

// authenticate a request and call the wrapped handler if authentication is successful
// if an empty authentication token was provided then we will not do any authenticaion
func (self AuthenticationMiddleware) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	// token value provided by the user that we want to authenticate
	// this value is provided as a bearer token in the http request header
	var userToken string

	// only bother getting the user token if the authentication token was set
	if len(self.Token) > 0 {
		// regular expression for matching a bearer token
		var tokenRegex = regexp.MustCompile("[Bb]earer (.*)")

		// get the authentication value the user provided in the http request
		var authValue = request.Header.Get("Authorization")

		// use the regular expression to check if the user token is in the format we are expecting
		var regexMatches = tokenRegex.FindStringSubmatch(authValue)
		// FindStringSubmatch returns a list of values on successful matching
		// value 0 will be the whole string passed in
		// subsequent values will be capture group values
		if len(regexMatches) > 0 {
			// since we provided a capture group in the token regex
			// and we know that the regex matched something
			// we know that regexMatches[1] is our matched token
			userToken = regexMatches[1]
		}
	}

	// if authentication was successful then call the next http handler
	// if authentication was not successful then send back a 401 response
	if userToken == self.Token {
		self.Handler.ServeHTTP(writer, request)
	} else {
		var err = DefaultHttpError(http.StatusUnauthorized)

		WriteJsonResponse(writer, err)
	}
}

// logging middleware to log each time there is a new request
type LoggingMiddleware struct {
	Logger  *log.Logger
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

// http handler router that can be used to register (and dispatch to) handlers for specific http methods
type MethodRouter struct {
	routes map[string]http.Handler
}

func NewMethodRouter() MethodRouter {
	var routes = make(map[string]http.Handler)

	return MethodRouter{
		routes: routes,
	}
}

func (self MethodRouter) Handle(method string, handler http.Handler) {
	if len(method) > 0 {
		self.routes[method] = handler
	}
}

func (self MethodRouter) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	var handler, routeIsRegistered = self.routes[request.Method]

	// if a handler has been registered for the requested method then we will
	// dispatch to that specific handler
	// if the method has NOT been registered then we will respond with a 405 Method Not Allowed
	if routeIsRegistered {
		handler.ServeHTTP(writer, request)
	} else {
		var err = DefaultHttpError(http.StatusMethodNotAllowed)

		WriteJsonResponse(writer, err)
	}
}

func EventsAddHandler(schema jsonschema.Schema) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// TODO
	})
}

func EventsQueryHandler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// TODO
	})
}

func main() {
	// set the logger to log messages in UTC time
	log.SetFlags(log.LstdFlags | log.LUTC)

	log.Println("Server starting")

	// variables that will be set to values supplied by the user via the command line
	var serverPort int
	var shouldServeTls bool
	var apiToken string

	flag.IntVar(&serverPort, "p", 80, "The TCP port for the server to listen on")
	flag.BoolVar(&shouldServeTls, "t", false, "Handle requests using TLS encryption")

	// TODO change this to a more sophisticated authentication method
	// ideally each user will have their own token so that access can be controlled more easily
	// NOTICE: an empty token means no authentication will be done
	flag.StringVar(&apiToken, "api-token", "", "Unique value used to authenticate users")

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

	var schemaFilePath = os.Getenv("AUDIT_LOG_EVENT_SCHEMA_FILE")
	if len(schemaFilePath) == 0 {
		log.Fatalf("A path to a json schema file for audit log events was not provided. Please provide on using the AUDIT_LOG_EVENT_SCHEMA_FILE environment variable")
	}

	var startupError error

	// open the json schema file for reading
	var fileReader io.Reader
	fileReader, startupError = os.Open(schemaFilePath)
	if startupError != nil {
		log.Fatalf("An error occured while reading the audit log event json schema file: %s", startupError)
	}

	// create a json schema object that will be used to validate event format
	var eventJsonSchema jsonschema.Schema
	// read the json schema into the schema object
	startupError = json.NewDecoder(fileReader).Decode(&eventJsonSchema)
	if startupError != nil {
		log.Fatalf("An error occured while parsing the audit log event json schema file: %s", startupError)
	}

	// create a new http multiplexer for handling http requests
	var mux = http.NewServeMux()

	// create a new method router so we can group similar operations for events to one endpoint path
	var eventsRouter = NewMethodRouter()
	// add the ability to ADD events to the event router
	eventsRouter.Handle(http.MethodPost, EventsAddHandler(eventJsonSchema))
	// add the ability to QUERY events to the event router
	eventsRouter.Handle(http.MethodGet, EventsQueryHandler())

	// add the audit log events router to the multiplexer
	mux.Handle("/events", eventsRouter)

	// TODO probably need GET PUT DELETE /events/<event>

	// the http handler that will be used to serve http requests
	var serveHandler http.Handler = mux

	// wrap the multiplexer in a middleware handler that logs when reqests are made
	serveHandler = LoggingMiddleware{
		Logger:  log.Default(),
		Handler: serveHandler,
	}

	// wrap the multiplexer in a middleware handler that authenticates requests
	serveHandler = AuthenticationMiddleware{
		Token:   apiToken,
		Handler: serveHandler,
	}

	// create an http server for serving requests using the wrapped multiplexer we created
	var server = http.Server{
		Addr:    fmt.Sprintf(":%d", serverPort),
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
