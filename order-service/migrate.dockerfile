# Образ с goose и миграциями. Строка подключения — из окружения:
# GOOSE_DRIVER=postgres, GOOSE_DBSTRING=<DATABASE_URL>.
FROM golang:1.26 AS build
RUN CGO_ENABLED=0 go install github.com/pressly/goose/v3/cmd/goose@v3.29.0

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /go/bin/goose /goose
COPY migrations/goose /migrations
ENV GOOSE_DRIVER=postgres GOOSE_MIGRATION_DIR=/migrations
ENTRYPOINT ["/goose"]
CMD ["up"]
