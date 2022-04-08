FROM golang:1.18-alpine3.15 AS base

COPY . /go/src/auditlog
WORKDIR /go/src/auditlog

RUN apk add git && \
	go get -d -v ./... && \
	go install -v ./...

FROM alpine:3.15

RUN mkdir /usr/lib/auditlog

# copy the auditlog resources and binary to the new build
COPY --from=base /go/src/auditlog/resources/* /usr/lib/auditlog/
COPY --from=base /go/bin/auditlog /usr/bin/

EXPOSE 80/tcp

ENV AUDIT_LOG_EVENT_SCHEMA_FILE=/usr/lib/auditlog/events_schema.json

CMD ["auditlog"]
