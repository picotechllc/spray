# Deploying the Spray Website

This guide explains how to deploy the Spray website to Google Cloud Storage and serve it using Spray itself.

## Overview

The website will be deployed to spray.picote.ch using the following architecture:
1. Static files are uploaded to a GCS bucket
2. Spray serves the static files from the bucket
3. DNS points to the Spray server

## Prerequisites

- Google Cloud SDK (`gcloud`) installed and configured
- A GCS bucket for hosting the website files
- A domain name (spray.picote.ch) pointing to your Spray server

## Deployment Steps

### 1. Upload Website Files to GCS

```bash
# Create a bucket (if it doesn't exist)
gsutil mb gs://spray.picote.ch

# Enable public access for static website hosting
gsutil iam ch allUsers:objectViewer gs://spray.picote.ch

# Upload the website files
gsutil -m cp -r website/* gs://spray.picote.ch/

# Verify the upload
gsutil ls gs://spray.picote.ch/
```

### 2. Configure Spray Server

Set up your Spray server with the following environment variables:

```bash
export BUCKET_NAME=spray.picote.ch
export GOOGLE_PROJECT_ID=shared-k8s-prd
export PORT=8080
```

### 3. Deploy Spray Server

**Using Docker:**

```bash
docker run -d \
  --name spray-server \
  -e BUCKET_NAME=spray.picote.ch \
  -e GOOGLE_PROJECT_ID=shared-k8s-prd \
  -p 8080:8080 \
  --restart unless-stopped \
  spray:latest
```

**Using Docker Compose:**

```yaml
version: '3.8'
services:
  spray:
    image: spray:latest
    ports:
      - "8080:8080"
    environment:
      - BUCKET_NAME=spray.picote.ch
      - GOOGLE_PROJECT_ID=shared-k8s-prd
    restart: unless-stopped
```

**On Google Cloud Run:**

```bash
gcloud run deploy spray-website \
  --image gcr.io/shared-k8s-prd/spray \
  --platform managed \
  --region us-central1 \
  --allow-unauthenticated \
  --set-env-vars BUCKET_NAME=spray.picote.ch,GOOGLE_PROJECT_ID=shared-k8s-prd
```

### 4. Configure DNS

Point your domain to the Spray server:

```
spray.picote.ch A    YOUR_SERVER_IP
```

Or if using Cloud Run:
```
spray.picote.ch CNAME ghs.googlehosted.com
```

### 5. SSL/TLS Configuration

For production deployment, ensure HTTPS is configured:

- **Cloud Run**: Automatic HTTPS with managed certificates
- **Self-hosted**: Use a reverse proxy like nginx or traefik with Let's Encrypt
- **Load Balancer**: Configure Google Cloud Load Balancer with managed SSL

## File Structure

```
website/
├── index.html          # Main landing page
└── style.css          # Styles for the website

deployment/
├── deploy.md          # This deployment guide
├── deploy.sh          # Bash deployment script
├── deploy.ps1         # PowerShell deployment script
└── README.md          # Deployment documentation
```

## Updating the Website

To update the website content:

1. Modify the files in the `website/` directory
2. Upload the changes to GCS:
   ```bash
   gsutil -m cp -r website/* gs://spray.picote.ch/
   ```
3. The changes will be live immediately (no server restart needed)

## Monitoring

Once deployed, you can monitor the website using:

- **Health Checks**: `https://spray.picote.ch/livez` and `https://spray.picote.ch/readyz`
- **Metrics**: `https://spray.picote.ch/metrics` (Prometheus format)
- **Logs**: Check your logging configuration for request logs

## Custom Redirects

If you need custom redirects, create a `.spray/redirects.toml` file in your bucket:

```toml
[redirects]
"/old-path" = "https://spray.picote.ch/new-path"
"/docs" = "https://github.com/picotechllc/spray"
```

## Security Considerations

- Ensure the GCS bucket has appropriate permissions
- Use HTTPS in production
- Consider setting up a CDN (Cloud CDN) for better performance
- Monitor access logs for security issues

## Performance Optimization

- Enable gzip compression in your reverse proxy
- Set appropriate cache headers for static assets
- Use a CDN for global distribution
- Monitor metrics for performance bottlenecks 