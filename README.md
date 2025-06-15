[![Coverage Status](https://coveralls.io/repos/github/picotechllc/spray/badge.svg?branch=main)](https://coveralls.io/github/picotechllc/spray?branch=main)

# Spray

Spray is a minimal Go web server that serves the contents of a Google Cloud Platform (GCP) bucket.

## Features

- Simple and lightweight
- Serves static files from a GCP bucket
- Easy to configure and deploy
- Reasonably comprehensive Prometheus metrics
- Custom redirects support
- Prometheus metrics
- Health check endpoints
- Configurable port

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

## Configuration

### Environment Variables

- `BUCKET_NAME`: The name of the GCS bucket to serve files from
- `GOOGLE_PROJECT_ID`: Your Google Cloud project ID
- `PORT`: (Optional) The port to listen on (default: 8080)

### Custom Redirects

You can configure custom redirects by creating a `.spray/redirects.toml` file in your GCS bucket. The file should be in TOML format:

```toml
[redirects]
"/old-path" = "https://example.com/new-path"
"/another-path" = "https://example.com/destination"
```

The redirects will take precedence over any files that might exist at the same path. The server will return a 302 Found response with the destination URL.

## Endpoints

- `/`: Serves static files from the GCS bucket
- `/metrics`: Prometheus metrics endpoint
- `/readyz`: Readiness probe endpoint
- `/livez`: Liveness probe endpoint

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

## Website

The Spray project includes a website that explains the project and demonstrates its capabilities. The website is hosted using Spray itself at [spray.picote.ch](https://spray.picote.ch).

### Directory Structure

- `website/` - Static website content (HTML, CSS)
- `deployment/` - Deployment scripts and documentation for the website

To deploy the website, see the documentation in the `deployment/` directory.

## Contact

For any questions or suggestions, please avail yourself of the GitHub Issues system.
