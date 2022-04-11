package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/mitchellkelly/auditlog/api"
	// TODO the custom http mux code (middlewares and routers) could be replaced
	// with a more sophisticated mux package (i prefer github.com/gorilla/mux)
	// the custom code is used here so that this service can mostly use features
	// already available in Go
	"github.com/mitchellkelly/auditlog/mux"
	"github.com/qri-io/jsonschema"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

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
	eventsRouter.Handle(http.MethodPost, api.EventsAddHandler(dbCollection, &eventJsonSchema))
	// add the ability to QUERY events to the event router
	eventsRouter.Handle(http.MethodGet, api.EventsQueryHandler(dbCollection))

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
