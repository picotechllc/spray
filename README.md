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
    go build
    ```

## Usage

Build the Docker image:
```sh
docker build -t spray .
```

Run the Docker container:
```sh
docker run -e BUCKET_NAME=my-bucket -p 8080:8080 spray
```

The server will start and serve the contents of my-bucket bucket on port 8080.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE.md) file for details.

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

## Contact

For any questions or suggestions, please avail yourself of the GitHub Issues system.
