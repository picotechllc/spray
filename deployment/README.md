# Spray Website

This directory contains the static website for the Spray project, which will be deployed to **spray.picote.ch**.

## Overview

Spray is a minimal Go web server that serves static files from Google Cloud Storage buckets. This website serves as the project's landing page and documentation.

## Files

**Website Content (`../website/`):**
- `index.html` - Main landing page with project overview and documentation
- `style.css` - Modern, responsive CSS styling

**Deployment Tools (`./deployment/`):**
- `deploy.md` - Detailed deployment instructions
- `deploy.sh` - Bash deployment script (Linux/macOS)
- `deploy.ps1` - PowerShell deployment script (Windows)
- `README.md` - This file

## Features

The website includes:

- **Hero Section**: Eye-catching introduction to Spray
- **Features Grid**: Key benefits and capabilities
- **Getting Started**: Step-by-step setup instructions
- **Documentation**: Configuration and endpoint reference
- **Responsive Design**: Works on desktop and mobile devices
- **Modern UI**: Clean, professional design with smooth animations

## Quick Deploy

Run these commands from the `deployment/` directory:

### Using PowerShell (Windows)
```powershell
cd deployment
./deploy.ps1
```

### Using Bash (Linux/macOS)
```bash
cd deployment
./deploy.sh
```

### Manual Deploy
```bash
# Set environment variables
export BUCKET_NAME=spray.picote.ch
export GOOGLE_PROJECT_ID=shared-k8s-prd

# Upload files
gsutil -m cp -r website/* gs://spray.picote.ch/
```

## Architecture

The website deployment uses the following architecture:

1. **Static Files**: HTML, CSS, and other assets are stored in a GCS bucket
2. **Spray Server**: Serves the files from the bucket with production features
3. **Domain**: spray.picote.ch points to the Spray server
4. **CDN**: Optional Cloud CDN for global performance

## Development

To modify the website:

1. Edit the HTML/CSS files in this directory
2. Test locally by opening `index.html` in a browser
3. Deploy using one of the deployment scripts
4. Changes are live immediately (no server restart needed)

## Monitoring

Once deployed, monitor the website at:

- **Website**: https://spray.picote.ch
- **Health Check**: https://spray.picote.ch/livez
- **Metrics**: https://spray.picote.ch/metrics

## Self-Hosting Example

This website is a perfect example of Spray's capabilities - it's hosted by Spray itself! The same server that serves this documentation is built using the Spray project.

## Contributing

To update the website:

1. Make your changes to the files in this directory
2. Test your changes locally
3. Run the deployment script to publish
4. Submit a pull request with your changes

## License

This website and the Spray project are licensed under the MIT License. 