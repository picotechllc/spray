[![Coverage Status](https://coveralls.io/repos/github/picotechllc/spray/badge.svg?branch=main)](https://coveralls.io/github/picotechllc/spray?branch=main)

# Spray

Spray is a minimal Go web server that serves the contents of a Google Cloud Platform (GCP) bucket.

## Features

- Simple and lightweight
- Serves static files from a GCP bucket
- Easy to configure and deploy
- Reasonably comprehensive Prometheus metrics

### Metrics

The following Prometheus metrics are exposed at `/metrics`:

- `gcs_server_requests_total` - Total number of HTTP requests processed, labeled by bucket, path, method and status code
- `gcs_server_request_duration_seconds` - HTTP request duration histogram, labeled by bucket, path and method
- `gcs_server_bytes_transferred_total` - Total bytes transferred, labeled by bucket, path, method and direction
- `gcs_server_active_requests` - Number of requests currently being processed, labeled by bucket
- `gcs_server_cache_total` - Cache hit/miss counter (reserved for future use)
- `gcs_server_errors_total` - Total number of errors encountered, labeled by bucket, path and error type
- `gcs_server_object_size_bytes` - Size of objects served, labeled by bucket and path
- `gcs_server_storage_operation_duration_seconds` - Latency of GCS operations, labeled by bucket and operation

These metrics provide visibility into:
- Request volume and latency
- Error rates and types
- Resource utilization
- GCS operation performance


## Development

This project uses [Just](https://github.com/casey/just) as a command runner. To get started:

1. Install Just:
   ```bash
   # macOS
   brew install just

   # Linux
   curl --proto '=https' --tlsv1.2 -sSf https://just.systems/install.sh | bash

   # Windows
   choco install just
   ```

2. Install development dependencies:
   ```bash
   just setup
   ```

3. View available commands:
   ```bash
   just
   ```

### Common Commands

- `just build` - Build the binary
- `just test` - Run tests with race detection and coverage
- `just coverage` - Run tests and view coverage report in browser
- `just fmt` - Format code
- `just lint` - Run linters
- `just check` - Run all pre-commit checks (format, lint, test)
- `just docker-build` - Build Docker image
- `just docker-run` - Run Docker container (requires credentials)

For local development with Docker:
```bash
# Build and run with Docker
just docker-run
```

## Installation

1. Clone the repository:
    ```sh
    git clone https://github.com/yourusername/spray.git
    ```
2. Navigate to the project directory:
    ```sh
    cd spray
    ```
3. Build the project:
    ```sh
    just build
    ```

## Usage

Build the Docker image:
```sh
just docker-build
```

Run the Docker container:
```sh
just docker-run
```

The server will start and serve the contents of my-bucket bucket on port 8080.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE.md) file for details.

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

## Contact

For any questions or suggestions, please avail yourself of the GitHub Issues system.
