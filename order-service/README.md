## Local Development

```bash
# Tests
go test ./...

# Race detector  
go test -race ./...

# Lint
golangci-lint run

# Security
govulncheck ./...

# Build
go build -o server ./cmd

# Docker
docker build .

# Run all
docker compose up --build
```