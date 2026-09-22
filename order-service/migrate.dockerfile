FROM golang:1.26
RUN go install github.com/pressly/goose/v3/cmd/goose@latest
WORKDIR /migrations
COPY migrations/goose/ .
ENV GOOSE_DRIVER=postgres
CMD goose up
