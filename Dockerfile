# Use the official Golang image to create a build artifact.
FROM golang:1.24.4 AS builder

# Accept version as build argument
ARG VERSION=docker-dev

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source from the current directory to the Working Directory inside the container
COPY . .

# Build the Go app with version information
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-X main.Version=${VERSION}" -o spray .

# Start a new stage from scratch
FROM gcr.io/distroless/static-debian12

# Set the Current Working Directory inside the container
WORKDIR /root/

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/spray .

# Expose port 8080 to the outside world
EXPOSE 8080

ENV BUCKET_NAME="spray-test.picote.ch"

# Command to run the executable
CMD ["/root/spray"]