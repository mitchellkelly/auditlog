# Audit Log Coding Exercise

Audit Log accepts event data sent by other systems and provides an endpoint to query the stored data.

The schema describing the events that this service will accept is defined in [the event schema](resources/events_schema.json).

This service uses the NoSQL database Mongo DB to store events. NoSQL (and Mongo DB in particular) is great when working with loosely structured data. Mongo uses a data-oriented data model that allows for data in different formats to be added without having to make any changes to the database.

One of the potential drawbacks of Mongo for this application is that Mongo only supports one main node. The main Mongo node is the only node that can accept input, so write scalability is a potential issue.

## Endpoints

Endpoint | Method
--- | ---
[/events](#post-events) | POST
[/events](#get-events) | GET

---

#### POST /events
Add a new event to the audit log.

This endpoint requires an http body that matches the event schema mentioned above.

#### GET /events
Get audit log events

This endpoint gets all of the audit log events that match the filter parameters.

If no filter parameters are provided, then all of the events will be returned.

Filter parameters can be provided as part of the URL query parameters as one or more key=value pairs.

---

## Authentication
The service will authenticate requests if an API token is provided via the AUDIT_LOG_API_TOKEN environment variable.

If an API token is provided to the audit log service, then all unauthenticated requests will result in a 401 Unauthorized response from the service.

To send an authenticated request, use the same API token that was provided to the service in the http 'Authorization' header as a bearer token. In cURL, you would add it like the following:

```
curl --header "Authorization: Bearer $AUDIT_LOG_API_TOKEN"
```

---

## Running

After cloning the repo and cd'ing into auditlog, the service can easily be run using Docker and Docker Compose.
```
# build the docker container
docker build -t mitchellkelly/auditlog .

# create a token that the API can use to authenticate requests
export AUDIT_LOG_API_TOKEN=$(uuidgen)

# deploy with docker-compose
docker-compose up -d
```

As a one-line command:
```
docker build -t mitchellkelly/auditlog . && export AUDIT_LOG_API_TOKEN=$(uuidgen) && docker-compose up -d
```

To stop the containers:
```
docker-compose down
```

By default, the service runs on port 80. This can be changed by providing the `-p` flag when starting the service.

The service can use TLS encryption if the `-t` flag is provided along with both the `AUDIT_LOG_TLS_CERT` and the `AUDIT_LOG_TLS_KEY` environment variables.

The service will try to connect to a Mongo database on localhost using port 27017 with no authentication.  
The service can connect to a different Mongo database by providing the `AUDIT_LOG_DB_HOST` and `AUDIT_LOG_DB_PORT` environment variables.  
Authentication can be used by providing the `AUDIT_LOG_DB_USERNAME` and `AUDIT_LOG_DB_PASSWORD` environment variables.

---

## Request examples

#### Adding data

A new user was created.
```
curl --header "Authorization: Bearer $AUDIT_LOG_API_TOKEN" http://localhost:8080/events -d '{"timestamp":1649445988, "summary":"A customer was added", "source":{"service_name":"customer-management", "service_version":"1.0.0"}, "attributes":{"customer_id":"c64c9e8c-e4e0-4569-859b-c9199ef92d55", "customer_name":"mitchell"}}'
```

A customer performed an action on a resource.
```
curl --header "Authorization: Bearer $AUDIT_LOG_API_TOKEN" http://localhost:8080/events -d '{"timestamp":1649451138, "summary":"A customer updated their profile", "source":{"service_name":"profile-service", "service_version":"1.4.2"}, "attributes":{"customer_id":"c64c9e8c-e4e0-4569-859b-c9199ef92d55", "profile_id": "f3180b5e-fd71-46b9-9a40-d30e73e8ffbd"}}'
```

A customer was billed.
```
curl --header "Authorization: Bearer $AUDIT_LOG_API_TOKEN" http://localhost:8080/events -d '{"timestamp":1649451262, "summary":"A customer was billed", "source":{"service_name":"billing-service", "service_version":"1.2.7"}, "attributes":{"customer_id":"c64c9e8c-e4e0-4569-859b-c9199ef92d55", "amount_billed": 8.99}}'
```

A customer was deactivated.
```
curl --header "Authorization: Bearer $AUDIT_LOG_API_TOKEN" http://localhost:8080/events -d '{"timestamp":1649451436, "summary":"A customer was deactivated", "source":{"service_name":"customer-management", "service_version":"1.0.0"}, "attributes":{"customer_id":"c64c9e8c-e4e0-4569-859b-c9199ef92d55", "reason":"Failure to pay"}}'
```

#### Querying data

Get all events.
```
curl --header "Authorization: Bearer $AUDIT_LOG_API_TOKEN" http://localhost:8080/events
```

Filtering on one field.
```
curl --header "Authorization: Bearer $AUDIT_LOG_API_TOKEN" http://localhost:8080/events?source.service_name=customer-management
```

Filtering on multiple fields.
```
curl --header "Authorization: Bearer $AUDIT_LOG_API_TOKEN" "http://localhost:8080/events?source.service_name=customer-management&attributes.customer_name=mitchell"
```
