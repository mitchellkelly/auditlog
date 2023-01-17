package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/mitchellkelly/auditlog/mux"
	"github.com/qri-io/jsonschema"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
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
			validationError, err = schema.ValidateBytes(request.Context(), d)
			// if something unexpected happened while validating the json we will just return a
			// simple 400 error
			// if the json body is invalid then we will return a 400 and a response body
			// describing why the json is invalid
			if err != nil {
				err = mux.DefaultHttpError(http.StatusBadRequest)
			} else {
				if len(validationError) > 0 {
					err = mux.HttpError{
						Code:        http.StatusBadRequest,
						Description: validationError.Error(),
					}
				}
			}
		}

		var event map[string]interface{}
		if err == nil {
			err = json.Unmarshal(d, &event)
		}

		if err == nil {
			// create a timed context to use when making requests to the db
			var timedContext, timedContextCancel = context.WithTimeout(request.Context(), 10*time.Second)

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
		var timedContext, timedContextCancel = context.WithTimeout(request.Context(), 10*time.Second)

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
			err = cursor.All(request.Context(), &results)
		}

		if err == nil {
			mux.WriteJsonResponse(writer, results)
		} else {
			mux.WriteJsonResponse(writer, err)
		}
	})
}
