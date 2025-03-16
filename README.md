[![Coverage Status](https://coveralls.io/repos/github/picotechllc/spray/badge.svg?branch=main)](https://coveralls.io/github/picotechllc/spray?branch=main)

# Spray

A simple HTTP server that serves files from a Google Cloud Storage bucket.

## Configuration

### Environment Variables

- `BUCKET_NAME` (required): The name of the GCS bucket to serve
- `GOOGLE_PROJECT_ID` (optional): GCP project ID for Cloud Logging. If not set, standard logging will be used
- `GOOGLE_APPLICATION_CREDENTIALS` (optional): Path to service account JSON file. See [Authentication](#authentication) below

### Command Line Flags

- `-port`: Server port (default: "8080")

## Authentication

Spray requires authentication to access Google Cloud Storage. There are several ways to provide credentials:

### 1. Running Locally

When running locally, you can authenticate using one of these methods:

a. **Application Default Credentials**
```bash
gcloud auth application-default login
```

b. **Service Account Key File**
```bash
export GOOGLE_APPLICATION_CREDENTIALS=/path/to/service-account.json
./spray
```

### 2. Running in Docker

When running in Docker, mount your credentials file and set the environment variable:

```bash
docker run -v /path/to/service-account.json:/credentials.json \
  -e GOOGLE_APPLICATION_CREDENTIALS=/credentials.json \
  -e BUCKET_NAME=your-bucket-name \
  ghcr.io/picotechllc/spray:latest
```

### 3. Running in Kubernetes

In Kubernetes, you have two options:

a. **Using Workload Identity** (recommended for GKE):
```yaml
apiVersion: v1
kind: Pod
metadata:
  annotations:
    iam.gke.io/gcp-service-account: YOUR_GSA_NAME@YOUR_PROJECT.iam.gserviceaccount.com
```

b. **Using a Service Account Key Secret**:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: gcp-credentials
type: Opaque
data:
  credentials.json: BASE64_ENCODED_SERVICE_ACCOUNT_KEY

---
apiVersion: v1
kind: Pod
metadata:
  name: spray
spec:
  containers:
  - name: spray
    image: ghcr.io/picotechllc/spray:latest
    env:
    - name: BUCKET_NAME
      value: "your-bucket-name"
    - name: GOOGLE_APPLICATION_CREDENTIALS
      value: "/credentials/credentials.json"
    volumeMounts:
    - name: gcp-credentials
      mountPath: "/credentials"
      readOnly: true
  volumes:
  - name: gcp-credentials
    secret:
      secretName: gcp-credentials
```

### Required Permissions

The service account needs the following IAM roles:
- `roles/storage.objectViewer` on the GCS bucket
- `roles/logging.logWriter` (optional, only if using Cloud Logging)

## Features

- Simple and lightweight
- Serves static files from a GCP bucket
- Easy to configure and deploy
- Comprehensive Prometheus metrics

## Metrics

Spray exports Prometheus metrics at `/metrics`. Each metric includes the `bucket_name` label to distinguish between instances serving different buckets.

### Available Metrics

- **Request Metrics**
  - `gcs_server_requests_total` - Total number of HTTP requests processed
    - Labels: bucket, path, method, status code
  - `gcs_server_request_duration_seconds` - HTTP request duration histogram
    - Labels: bucket, path, method
  - `gcs_server_active_requests` - Number of requests currently being processed
    - Labels: bucket

- **Data Transfer Metrics**
  - `gcs_server_bytes_transferred_total` - Total bytes transferred
    - Labels: bucket, path, method, direction
  - `gcs_server_object_size_bytes` - Size of objects served
    - Labels: bucket, path

- **Error Metrics**
  - `gcs_server_errors_total` - Total number of errors encountered
    - Labels: bucket, path, error type

- **Storage Metrics**
  - `gcs_server_storage_operation_duration_seconds` - Latency of GCS operations
    - Labels: bucket, operation
  - `gcs_server_cache_total` - Cache hit/miss counter (reserved for future use)
    - Labels: bucket, status

These metrics provide visibility into:
- Request patterns and performance
- Error rates and types
- Resource utilization
- Storage operation latency

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
