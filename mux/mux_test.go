package mux

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"testing"
)

var baseHandler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
	var statusCode = http.StatusOK

	writer.WriteHeader(statusCode)
	writer.Write([]byte(http.StatusText(statusCode)))
})

var writeJsonResponseInvalidStatusError = "An unexpected status code was returned when attempting to write a json response " +
	"Expected: %d, Got: %d"
var writeJsonResponseInvalidBodyError = "An unexpected response body was returned when attempting to write a json response " +
	"Expected: %s, Got: %s"

func TestWriteJsonResponseValidEmptyValue(t *testing.T) {
	// create a testing response writer so we can check the response
	// after the request finishes
	var writer testingResponseWriter

	WriteJsonResponse(&writer, nil)

	if writer.responseCode != http.StatusNoContent {
		t.Errorf(writeJsonResponseInvalidStatusError, http.StatusNoContent, writer.responseCode)
	}

	var expectedResponseText = "{}"
	if string(writer.responseText) != expectedResponseText {
		t.Errorf(writeJsonResponseInvalidBodyError, expectedResponseText, string(writer.responseText))
	}
}

func TestWriteJsonResponseValidSimpleValue(t *testing.T) {
	// create a testing response writer so we can check the response
	// after the request finishes
	var writer testingResponseWriter

	WriteJsonResponse(&writer, "123")

	if writer.responseCode != http.StatusOK {
		t.Errorf(writeJsonResponseInvalidStatusError, http.StatusOK, writer.responseCode)
	}

	var expectedResponseText = `"123"`
	if string(writer.responseText) != expectedResponseText {
		t.Errorf(writeJsonResponseInvalidBodyError, expectedResponseText, string(writer.responseText))
	}
}

func TestWriteJsonResponseValidStruct(t *testing.T) {
	// create a testing response writer so we can check the response
	// after the request finishes
	var writer testingResponseWriter

	var s = struct {
		One int
		Two string
	}{
		One: 1,
		Two: "two",
	}

	WriteJsonResponse(&writer, s)

	if writer.responseCode != http.StatusOK {
		t.Errorf(writeJsonResponseInvalidStatusError, http.StatusOK, writer.responseCode)
	}

	var expectedResponseText, _ = json.Marshal(s)
	if string(writer.responseText) != string(expectedResponseText) {
		t.Errorf(writeJsonResponseInvalidBodyError, string(expectedResponseText), string(writer.responseText))
	}
}

// struct that always returns an error when trying to mashal it to json
type invalidJsonStruct struct{}

func (self invalidJsonStruct) MarshalJSON() ([]byte, error) {
	return nil, fmt.Errorf("Nasty error")
}

func TestWriteJsonResponseInvalidStruct(t *testing.T) {
	// create a testing response writer so we can check the response
	// after the request finishes
	var writer testingResponseWriter

	var s = invalidJsonStruct{}
	WriteJsonResponse(&writer, s)

	if writer.responseCode != http.StatusInternalServerError {
		t.Errorf(writeJsonResponseInvalidStatusError, http.StatusInternalServerError, writer.responseCode)
	}

	var expectedResponseText, _ = json.Marshal(DefaultHttpError(http.StatusInternalServerError))
	if string(writer.responseText) != string(expectedResponseText) {
		t.Errorf(writeJsonResponseInvalidBodyError, expectedResponseText, string(writer.responseText))
	}
}

func TestWriteJsonResponseValidInternalError(t *testing.T) {
	// create a testing response writer so we can check the response
	// after the request finishes
	var writer testingResponseWriter

	var e = fmt.Errorf("Nasty error")

	WriteJsonResponse(&writer, e)

	if writer.responseCode != http.StatusInternalServerError {
		t.Errorf(writeJsonResponseInvalidStatusError, http.StatusInternalServerError, writer.responseCode)
	}

	var e2 = HttpError{
		Description: e.Error(),
	}

	var expectedResponseText, _ = json.Marshal(e2)
	if string(writer.responseText) != string(expectedResponseText) {
		t.Errorf(writeJsonResponseInvalidBodyError, expectedResponseText, string(writer.responseText))
	}
}

func TestWriteJsonResponseValidHttpError(t *testing.T) {
	// create a testing response writer so we can check the response
	// after the request finishes
	var writer testingResponseWriter

	var e = DefaultHttpError(http.StatusTeapot)

	WriteJsonResponse(&writer, e)

	if writer.responseCode != e.Code {
		t.Errorf(writeJsonResponseInvalidStatusError, e.Code, writer.responseCode)
	}

	var expectedResponseText, _ = json.Marshal(e)
	if string(writer.responseText) != string(expectedResponseText) {
		t.Errorf(writeJsonResponseInvalidBodyError, expectedResponseText, string(writer.responseText))
	}
}

var authRequestError = "An unexpected status code was returned when attempting to authenticate a request " +
	"Expected: %d, Got: %d"

func TestAuthenticationMiddlewareEmptyTokenSuccessAuth(t *testing.T) {
	// create an authentication middleware
	var aMiddleware = AuthenticationMiddleware{
		Token:   "",
		Handler: baseHandler,
	}

	// create a testing response writer so we can check the response status
	// after the request finishes
	var writer testingResponseWriter
	// create a request so we can add the auth header to it
	var request = http.Request{
		Header: http.Header{},
	}
	request.Header.Set("Authorization", "Bearer ")

	aMiddleware.ServeHTTP(&writer, &request)

	if writer.responseCode != http.StatusOK {
		t.Errorf(authRequestError, http.StatusOK, writer.responseCode)
	}
}

func TestAuthenticationMiddlewareEmptyTokenNoHeaderSuccessAuth(t *testing.T) {
	// create an authentication middleware
	var aMiddleware = AuthenticationMiddleware{
		Token:   "",
		Handler: baseHandler,
	}

	// create a testing response writer so we can check the response status
	// after the request finishes
	var writer testingResponseWriter

	aMiddleware.ServeHTTP(&writer, &http.Request{})

	if writer.responseCode != http.StatusOK {
		t.Errorf(authRequestError, http.StatusOK, writer.responseCode)
	}
}

func TestAuthenticationMiddlewareIncorrectTokenFailAuth(t *testing.T) {
	// create an authentication middleware
	var aMiddleware = AuthenticationMiddleware{
		Token:   "bhakrswqtqnspfqbclzn",
		Handler: baseHandler,
	}

	// create a testing response writer so we can check the response status
	// after the request finishes
	var writer testingResponseWriter
	// create a request so we can add the auth header to it
	var request = http.Request{
		Header: http.Header{},
	}
	request.Header.Set("Authorization", "Bearer 123")

	aMiddleware.ServeHTTP(&writer, &request)

	if writer.responseCode != http.StatusUnauthorized {
		t.Errorf(authRequestError, http.StatusUnauthorized, writer.responseCode)
	}
}

func TestAuthenticationMiddlewareIncorrectTokenEmptyTokenFailAuth(t *testing.T) {
	// create an authentication middleware
	var aMiddleware = AuthenticationMiddleware{
		Token:   "bhakrswqtqnspfqbclzn",
		Handler: baseHandler,
	}

	// create a testing response writer so we can check the response status
	// after the request finishes
	var writer testingResponseWriter
	// create a request so we can add the auth header to it
	var request = http.Request{
		Header: http.Header{},
	}
	request.Header.Set("Authorization", "Bearer ")

	aMiddleware.ServeHTTP(&writer, &request)

	if writer.responseCode != http.StatusUnauthorized {
		t.Errorf(authRequestError, http.StatusUnauthorized, writer.responseCode)
	}
}

func TestAuthenticationMiddlewareIncorrectTokenNoHeaderFailAuth(t *testing.T) {
	// create an authentication middleware
	var aMiddleware = AuthenticationMiddleware{
		Token:   "bhakrswqtqnspfqbclzn",
		Handler: baseHandler,
	}

	// create a testing response writer so we can check the response status
	// after the request finishes
	var writer testingResponseWriter

	aMiddleware.ServeHTTP(&writer, &http.Request{})

	if writer.responseCode != http.StatusUnauthorized {
		t.Errorf(authRequestError, http.StatusUnauthorized, writer.responseCode)
	}
}

func TestAuthenticationMiddlewareValidTokenNoBearerFailAuth(t *testing.T) {
	var aMiddleware = AuthenticationMiddleware{
		Token:   "bhakrswqtqnspfqbclzn",
		Handler: baseHandler,
	}

	// create a testing response writer so we can check the response status
	// after the request finishes
	var writer testingResponseWriter
	// create a request so we can add the auth header to it
	var request = http.Request{
		Header: http.Header{},
	}
	request.Header.Set("Authorization", "bhakrswqtqnspfqbclzn")

	aMiddleware.ServeHTTP(&writer, &request)

	if writer.responseCode != http.StatusUnauthorized {
		t.Errorf(authRequestError, http.StatusUnauthorized, writer.responseCode)
	}
}

func TestAuthenticationMiddlewareValidTokenLowercaseBearerHeaderSuccessAuth(t *testing.T) {

	var aMiddleware = AuthenticationMiddleware{
		Token:   "bhakrswqtqnspfqbclzn",
		Handler: baseHandler,
	}

	// create a testing response writer so we can check the response status
	// after the request finishes
	var writer testingResponseWriter
	// create a request so we can add the auth header to it
	var request = http.Request{
		Header: http.Header{},
	}
	request.Header.Set("Authorization", "bearer bhakrswqtqnspfqbclzn")

	aMiddleware.ServeHTTP(&writer, &request)

	if writer.responseCode != http.StatusOK {
		t.Errorf(authRequestError, http.StatusOK, writer.responseCode)
	}
}

func TestAuthenticationMiddlewareValidTokenUppercaseBearerHeaderSuccessAuth(t *testing.T) {
	var aMiddleware = AuthenticationMiddleware{
		Token:   "bhakrswqtqnspfqbclzn",
		Handler: baseHandler,
	}

	// create a testing response writer so we can check the response status
	// after the request finishes
	var writer testingResponseWriter
	// create a request so we can add the auth header to it
	var request = http.Request{
		Header: http.Header{},
	}
	request.Header.Set("Authorization", "Bearer bhakrswqtqnspfqbclzn")

	aMiddleware.ServeHTTP(&writer, &request)

	if writer.responseCode != http.StatusOK {
		t.Errorf(authRequestError, http.StatusOK, writer.responseCode)
	}
}

func TestLoggingMiddleware(t *testing.T) {
	var buf bytes.Buffer

	// create a logger that logs to the buffer so we can read from it later
	var logger = log.New(&buf, "", 0)
	// create a logging middleare to test
	var lMiddleware = LoggingMiddleware{
		Logger:  logger,
		Handler: baseHandler,
	}
	// test the middleware with a defualt writer and request
	lMiddleware.ServeHTTP(&testingResponseWriter{}, &http.Request{})

	// read the data in the buffer and make sure its not empty
	var loggedData, _ = ioutil.ReadAll(&buf)
	if len(loggedData) == 0 {
		t.Error("The logging middleware did not log any data")
	}
}

var methodRouterError = "An unexpected status code was returned when attempting to route a request " +
	"Expected: %d, Got: %d"

func TestMethodRouterServeValidRoute(t *testing.T) {
	// create a new method router
	var methodRouter = NewMethodRouter()
	// add a handler to it
	methodRouter.Handle(http.MethodGet, baseHandler)

	// create a testing response writer so we can check the response status
	// after the request finishes
	var writer testingResponseWriter

	var request = http.Request{
		Method: http.MethodGet,
	}

	// serve a request and check that it successfully hit a route
	methodRouter.ServeHTTP(&writer, &request)

	if writer.responseCode != http.StatusOK {
		t.Errorf(methodRouterError, http.StatusOK, writer.responseCode)
	}
}

func TestMethodRouterServeInvalidRoute(t *testing.T) {
	// create a new method router
	var methodRouter = NewMethodRouter()
	// add a handler to it
	methodRouter.Handle(http.MethodGet, baseHandler)
	methodRouter.Handle(http.MethodPost, baseHandler)
	methodRouter.Handle(http.MethodDelete, baseHandler)

	// create a testing response writer so we can check the response status
	// after the request finishes
	var writer testingResponseWriter

	var request = http.Request{
		Method: http.MethodPut,
	}

	// serve a request and check that it successfully hit a route
	methodRouter.ServeHTTP(&writer, &request)

	if writer.responseCode != http.StatusMethodNotAllowed {
		t.Errorf(methodRouterError, http.StatusMethodNotAllowed, writer.responseCode)
	}
}
