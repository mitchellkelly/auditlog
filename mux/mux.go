package mux

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
)

// WriteJsonResponse is a generic way of writing an http response with a json body
// the function determines what http status code to write based on the type of v
// if v is nil then the status code will be 204
// if v is an error the status code will either be HttpError.Code
// of a 500 if the the error is not of type HttpError
// if v is any non error value the function will attempt to marshal it to json
// and send a 200 and the json body to the user
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
				v = HttpError{
					Description: e.Error(),
				}

				statusCode = 500
			} else {
				statusCode = httpErr.Code
			}
		}

		var err error
		// marshal the response object into json so we can send it to the user
		responseBytes, err = json.Marshal(v)

		// if marshaling the json was successful then we will send the user provided status code if one was set
		// or a 200 if nothing was set by the user
		// if an error occured while marshaling the object to json then we will send a plain 500 error
		if err == nil {
			if statusCode == 0 {
				statusCode = http.StatusOK
			}
		} else {
			statusCode = http.StatusInternalServerError
			responseBytes = []byte(fmt.Sprintf(`{"description":"%s"}`, http.StatusText(statusCode)))
		}
	} else {
		// if v is nil then the user does not want to write anything
		// just send a 204 and an empty json object
		statusCode = http.StatusNoContent
		responseBytes = []byte{'{', '}'}
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(responseBytes)))
	writer.WriteHeader(statusCode)
	writer.Write(responseBytes)
}

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
// TODO using a single api token is not a very secure authentication method
// ideally the service would use a more dynamic authentication method like JWTs
func (self AuthenticationMiddleware) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	// token value provided by the user that we want to authenticate
	// this value is provided as a bearer token in the http request header
	var userToken string

	// regular expression for matching a bearer token
	var tokenRegex = regexp.MustCompile("^[Bb]earer (.+)$")

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

// create a new MethodRouter
func NewMethodRouter() MethodRouter {
	var routes = make(map[string]http.Handler)

	return MethodRouter{
		routes: routes,
	}
}

// add an http handler for the http method provided
func (self MethodRouter) Handle(method string, handler http.Handler) {
	if len(method) > 0 {
		self.routes[method] = handler
	}
}

// serve an http request if a handler has been defined for the method the user is requesting
// if no handler has been defined a 405 will be sent back to the user
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
