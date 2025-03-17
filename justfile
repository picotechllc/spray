# List available recipes
default:
    @just --list

# Run tests with race detection and coverage
test:
    go test -v -race -coverprofile=coverage.txt -covermode=atomic ./...

# Run tests and open coverage report in browser
coverage: test
    go tool cover -html=coverage.txt

# Build the binary
build:
    go build -o spray

# Run the server locally (requires GCP credentials)
run: build
    ./spray

# Build and run with Docker
docker-build:
    docker build -t spray .

# Run the Docker container (requires env file and credentials)
docker-run: docker-build
    docker run --env-file spray.env -v ${GOOGLE_APPLICATION_CREDENTIALS}:/credentials.json \
        -e GOOGLE_APPLICATION_CREDENTIALS=/credentials.json \
        -p 8080:8080 spray

# Format code
fmt:
    go fmt ./...

# Run linters
lint:
    golangci-lint run

# Clean build artifacts
clean:
    rm -f spray coverage.txt

# Install development dependencies
setup:
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Update dependencies
update:
    go get -u ./...
    go mod tidy

# Run all pre-commit checks
check: fmt lint test 