package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"time"

	// TODO the custom http mux code (middlewares and routers) could be replaced
	// with a more sophisticated mux package (i prefer github.com/gorilla/mux)
	// the custom code is used here so that this service can mostly use features
	// already available in Go
	"github.com/mitchellkelly/auditlog/mux"
	"github.com/qri-io/jsonschema"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ValidationError []jsonschema.KeyError

// create a string representation of the json schema errors
func (self ValidationError) Error() string {
	// string representation of all of the validation errors
	var validationErrorString string
	// one instance of a validation error string used to build the concatenated string
	var veString string

	// build a regular expression we can use to match / replace double quotes
	var quoteReplaceRegex = regexp.MustCompile("\"")

	for _, ve := range self {
		// validation errors occasionally use double quotes in their string values
		// we want to replace all instances of double quotes " with single quotes ' so that we can send the data
		// back to the user in a json string without it having a lot of escaped characters
		veString = string(quoteReplaceRegex.ReplaceAll([]byte(ve.Message), []byte{'\''}))
		// the PropertyPath is not always set or can be just /
		// if PropertyPath is a good value then we want to add it to the error string
		if len(ve.PropertyPath) != 0 && ve.PropertyPath != "/" {
			veString = fmt.Sprintf("%s %s", ve.PropertyPath, veString)
		}

		if len(validationErrorString) == 0 {
			// if the error string hasnt been set up yet the we want to
			// add a summary to the beginning
			validationErrorString = fmt.Sprintf("The json did not match the expected format: %s", veString)
		} else {
			// if the error string has been set up then we just want to add the next error on
			validationErrorString = fmt.Sprintf("%s; %s", validationErrorString, veString)
		}
	}

	return validationErrorString
}

// EventsAddHandler creates an http handler that validates and adds events to the database
func EventsAddHandler(db *mongo.Collection, schema *jsonschema.Schema) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// read the data from the request body
		var d, err = ioutil.ReadAll(request.Body)
		if err != nil {
			err = mux.DefaultHttpError(http.StatusBadRequest)
		}

		if err == nil {
			var validationError ValidationError
			// validate the request data using the json schema
			validationError, err = schema.ValidateBytes(context.Background(), d)
			// if something unexpected happened while validating the json we will just return a
			// simple 400 error
			// if the json body is invalid then we will return a 400 and a response body
			// describing why the json is invalid
			if err != nil {
				err = mux.DefaultHttpError(http.StatusBadRequest)
			} else {
				err = mux.HttpError{
					Code:        http.StatusBadRequest,
					Description: validationError.Error(),
				}
			}
		}

		var event map[string]interface{}
		if err == nil {
			err = json.Unmarshal(d, &event)
		}

		if err == nil {
			// create a timed context to use when making requests to the db
			var timedContext, timedContextCancel = context.WithTimeout(context.Background(), 10*time.Second)

			_, err = db.InsertOne(timedContext, event)
			// close the context to release any resources associated with it
			timedContextCancel()
		}

		mux.WriteJsonResponse(writer, err)
	})
}

func CreateFilterFromQuery(queryParams url.Values) map[string]interface{} {
	// create a filter object
	// we have to call make() because the collection.Find method assumes filter will be non nil
	var filter = make(map[string]interface{})

	for k, _ := range queryParams {
		var v interface{}

		// queryParams is a url.Values type which is map[string][]string
		// we want url.Values map key but we will call the url.Values.Get(k) method
		// since it returns a string
		var queryValueString = queryParams.Get(k)

		// handle id values as a special case
		// we want to query for a 24 character hex id
		// but mongo assumes we are using the 12 byte format
		if k == "_id" {
			var objectId, _ = primitive.ObjectIDFromHex(queryValueString)
			v = objectId
		} else {
			v = queryValueString
		}

		// trying to pass a string filter value for a non string data type results in no match
		// i.e. trying to filter for timestamp == "1648857887" will not match a row where timestamp == 1648857887
		// TODO allow for filtering of values other than strings
		// this could be done by using the jsonschema, checking the object type
		// and parsing it appropriately before adding it to the filter

		filter[k] = v
	}

	return filter
}

// EventsQueryHandler creates an http handler that retrieves values from the database
// optionally allowing to filter the vaules
func EventsQueryHandler(db *mongo.Collection) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// get a filter using the url query params
		var filter = CreateFilterFromQuery(request.URL.Query())

		// TODO allow the user to sort the response by providing a sort=<field> value in the query params

		// create a timed context to use when making requests to the db
		var timedContext, timedContextCancel = context.WithTimeout(context.Background(), 10*time.Second)

		// execute a find command against the db
		// this will return a cursor that we can request values from
		var cursor, err = db.Find(timedContext, filter, nil)
		// close the context to release any resources associated with it
		timedContextCancel()

		// results will be all of the events in the db that match the filter
		// if no filter is provided the all of the results will be returned
		// we set results to an intially empty list so that if the db returns 0 values
		// the endpoint will give the user an empty array instead of the nil json object
		var results = make([]map[string]interface{}, 0)
		if err == nil {
			// curse through all of the results and add them to the results list
			err = cursor.All(context.Background(), &results)
		}

		if err == nil {
			mux.WriteJsonResponse(writer, results)
		} else {
			mux.WriteJsonResponse(writer, err)
		}
	})
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

	// TODO change this to a more sophisticated authentication method
	// ideally each user will have their own token so that access can be controlled more easily
	// NOTICE: an empty token means no authentication will be done
	var apiToken = os.Getenv("AUDIT_LOG_API_TOKEN")

	var schemaFilePath = os.Getenv("AUDIT_LOG_EVENT_SCHEMA_FILE")
	if len(schemaFilePath) == 0 {
		log.Fatalf("A path to a json schema file for audit log events was not provided. Please provide on using the AUDIT_LOG_EVENT_SCHEMA_FILE environment variable")
	}

	var dbCredString string
	// get the db username and password from env variable
	var dbUsername = os.Getenv("AUDIT_LOG_DB_USERNAME")
	var dbPassword = os.Getenv("AUDIT_LOG_DB_PASSWORD")
	// if either vaule is empty then we will leave the credential string empty
	if len(dbUsername) != 0 && len(dbPassword) != 0 {
		dbCredString = fmt.Sprintf("%s@%s", dbUsername, dbPassword)
	}
	var dbHost = os.Getenv("AUDIT_LOG_DB_HOST")
	// get the db port from env variable
	// setting it to localhost if it is not provided
	if len(dbHost) == 0 {
		dbHost = "localhost"
	}
	// get the db port from env variable
	// setting it to the mongo default if it is not provided
	var dbPort = os.Getenv("AUDIT_LOG_DB_PORT")
	if dbPort == "" {
		dbPort = "27017"
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

	// create an options object to use to supply options when creating the db
	var dbConnectionString = fmt.Sprintf("mongodb://%s%s:%s", dbCredString, dbHost, dbPort)
	var dbClientOptions = options.Client().ApplyURI(dbConnectionString)

	// create a timed context to use when making requests to the db
	var timedContext, timedContextCancel = context.WithTimeout(context.Background(), 10*time.Second)

	var dbClient *mongo.Client
	// connect to db
	dbClient, startupError = mongo.Connect(timedContext, dbClientOptions)
	if startupError != nil {
		log.Fatalf("An error occured while connecting to the database: %s", startupError)
	}
	// cancel the timed context to release any resources associated with it
	timedContextCancel()

	// create a new timed context to use to test the db connection
	timedContext, timedContextCancel = context.WithTimeout(context.Background(), 10*time.Second)
	// test the db connection
	startupError = dbClient.Ping(timedContext, nil)
	if startupError != nil {
		log.Fatalf("An error occured while verifying the connection to the database: %s", startupError)
	}

	// connect to the 'auditlog' db 'event' collection
	var dbCollection = dbClient.Database("auditlog").Collection("event")

	// create a new http multiplexer for handling http requests
	var muliplexer = http.NewServeMux()

	// create a new method router so we can group similar operations for events to one endpoint path
	var eventsRouter = mux.NewMethodRouter()
	// add the ability to ADD events to the event router
	eventsRouter.Handle(http.MethodPost, EventsAddHandler(dbCollection, &eventJsonSchema))
	// add the ability to QUERY events to the event router
	eventsRouter.Handle(http.MethodGet, EventsQueryHandler(dbCollection))

	// add the audit log events router to the multiplexer
	muliplexer.Handle("/events", eventsRouter)

	// TODO probably need GET PUT DELETE /events/<event>
	// TODO probably need GET /health

	// the http handler that will be used to serve http requests
	var serveHandler http.Handler = muliplexer

	// wrap the multiplexer in a middleware handler that logs when reqests are made
	serveHandler = mux.LoggingMiddleware{
		Logger:  log.Default(),
		Handler: serveHandler,
	}

	// wrap the multiplexer in a middleware handler that authenticates requests
	serveHandler = mux.AuthenticationMiddleware{
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
