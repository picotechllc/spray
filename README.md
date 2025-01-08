# Spray

Spray is a minimal Go web server that serves the contents of a Google Cloud Platform (GCP) bucket.

## Features

- Simple and lightweight
- Serves static files from a GCP bucket
- Easy to configure and deploy

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

## Configuration

Create a configuration file `config.json` with the following structure:
```json
{
    "bucketName": "your-gcp-bucket-name",
    "port": "8080"
}
```

## Usage

Run the server:
```sh
./spray
```

The server will start and serve the contents of the specified GCP bucket on the configured port.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE.md) file for details.

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

## Contact

For any questions or suggestions, please avail yourself of the GitHub Issues system.
