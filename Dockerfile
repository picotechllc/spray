# Use the official Golang image to create a build artifact.
FROM golang:1.23.4 AS builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source from the current directory to the Working Directory inside the container
COPY . .

# Build the Go app
RUN go build -o spray .

# Start a new stage from scratch
FROM alpine:latest

# Install ca-certificates
RUN apk --no-cache add ca-certificates

# Set the Current Working Directory inside the container
WORKDIR /root/

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/spray .

# Set the bucket name as a build argument
ARG BUCKET_NAME

# Create the config.json file dynamically
RUN echo "{\"bucketName\": \"${BUCKET_NAME}\", \"port\": \"8080\"}" > config.json

# Expose port 8080 to the outside world
EXPOSE 8080

# Command to run the executable
CMD ["./spray"]