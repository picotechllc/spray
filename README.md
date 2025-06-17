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

### Inspecting Redirect Configuration

You can inspect the current redirect configuration of a running Spray instance by accessing the `/config/redirects` endpoint. This returns a JSON response with the following structure:

```json
{
  "redirects": {
    "/old-path": "https://example.com/new-path",
    "/github": "https://github.com/picotechllc/spray"
  },
  "count": 2,
  "config_source": ".spray/redirects.toml",
  "bucket_name": "your-bucket-name"
}
```

This endpoint is useful for:
- Debugging redirect issues
- Verifying configuration changes have been applied
- Monitoring redirect rules in production
- Integration with configuration management tools

## X-Powered-By Header Configuration

Spray automatically adds an `X-Powered-By` header to all responses, which can be customized using a hybrid approach that gives both server administrators and site owners control.

### Server Administrator Control (Environment Variables)

Server administrators can control the X-Powered-By header using the `SPRAY_POWERED_BY_HEADER` environment variable:

```bash
# Default behavior (if not set) - shows version
# Results in: X-Powered-By: spray/v1.0.0
# (no environment variable needed)

# Custom branding
export SPRAY_POWERED_BY_HEADER="MyCompany-CDN/spray"
# Results in: X-Powered-By: MyCompany-CDN/spray

# Disable entirely (site owners cannot override this)
export SPRAY_POWERED_BY_HEADER=""
# Results in: No X-Powered-By header
```

### Site Owner Control (.spray/headers.toml)

Site owners can opt out of the X-Powered-By header by creating a `.spray/headers.toml` file in their bucket:

```toml
[powered_by]
enabled = false
```

**Important Notes:**
- Site owners can only **disable** the header, not change its value
- If the server administrator disables the header via environment variable, site owners cannot re-enable it
- If no `headers.toml` file exists, the header is enabled by default
- This configuration file supports future extensibility for other header controls

### Precedence Rules

The header behavior follows this precedence:

1. **Environment variable empty** → No header (site owners cannot override)
2. **Environment variable has value** AND **no headers.toml** → Use environment variable value  
3. **Environment variable has value** AND **headers.toml disables it** → No header
4. **Environment variable has value** AND **headers.toml enables it** → Use environment variable value

This hybrid approach ensures:
- **Security**: Server admins control what version information is exposed
- **Flexibility**: Site owners can opt out without requiring server changes  
- **Performance**: Minimal overhead with sensible defaults

## Endpoints

- `/`: Serves static files from the GCS bucket
- `/metrics`: Prometheus metrics endpoint
- `/readyz`: Readiness probe endpoint
- `/livez`: Liveness probe endpoint
- `/config/redirects`: Returns the current redirect configuration as JSON

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
