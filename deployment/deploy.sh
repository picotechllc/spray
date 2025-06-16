#!/bin/bash

# Static Website Deployment Script for Spray
# This script uploads the website files to a Google Cloud Storage bucket

set -e

# Check for --spray-project flag
USE_SPRAY_DEFAULTS=false
if [[ "$1" == "--spray-project" ]]; then
    USE_SPRAY_DEFAULTS=true
    shift # Remove the flag from arguments
fi

# Configuration - Set these environment variables or use --spray-project flag
if [ "$USE_SPRAY_DEFAULTS" = true ]; then
    BUCKET_NAME="${BUCKET_NAME:-spray.picote.ch}"
    PROJECT_ID="${GOOGLE_PROJECT_ID:-shared-k8s-prd}"
    echo "🎯 Using Spray project defaults"
else
    BUCKET_NAME="${BUCKET_NAME:-}"
    PROJECT_ID="${GOOGLE_PROJECT_ID:-}"
fi

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}🚀 Static Website Deployment Script${NC}"
echo "===================================="

# Check if required tools are installed
if ! command -v gsutil &> /dev/null; then
    echo -e "${RED}❌ Error: gsutil is not installed. Please install Google Cloud SDK.${NC}"
    exit 1
fi

# Check if bucket name is provided
if [ -z "$BUCKET_NAME" ]; then
    echo -e "${RED}❌ Error: BUCKET_NAME is required.${NC}"
    echo "Options:"
    echo "  1. Set environment variable: export BUCKET_NAME=your-bucket-name"
    echo "  2. Pass when running: BUCKET_NAME=your-bucket-name ./deploy.sh"
    echo "  3. Use Spray project defaults: ./deploy.sh --spray-project"
    exit 1
fi

# Check if project ID is set
if [ -z "$PROJECT_ID" ]; then
    if [ "$USE_SPRAY_DEFAULTS" = false ]; then
        echo -e "${YELLOW}⚠️  Warning: GOOGLE_PROJECT_ID not set. Using current gcloud project.${NC}"
    fi
    PROJECT_ID=$(gcloud config get-value project 2>/dev/null || echo "")
    if [ -z "$PROJECT_ID" ]; then
        echo -e "${RED}❌ Error: No Google Cloud project configured.${NC}"
        echo "Options:"
        echo "  1. Set environment variable: export GOOGLE_PROJECT_ID=your-project-id"
        echo "  2. Configure gcloud: gcloud config set project your-project-id"
        if [ "$USE_SPRAY_DEFAULTS" = false ]; then
            echo "  3. Use Spray project defaults: ./deploy.sh --spray-project"
        fi
        exit 1
    fi
fi

echo "📋 Configuration:"
echo "   Bucket: $BUCKET_NAME"
echo "   Project: $PROJECT_ID"
echo ""

# Check if bucket exists
echo "🔍 Checking if bucket exists..."
if ! gsutil ls -b "gs://$BUCKET_NAME" &> /dev/null; then
    echo -e "${YELLOW}📦 Bucket doesn't exist. Creating gs://$BUCKET_NAME...${NC}"
    gsutil mb -p "$PROJECT_ID" "gs://$BUCKET_NAME"
    
    echo "🔓 Making bucket publicly readable..."
    gsutil iam ch allUsers:objectViewer "gs://$BUCKET_NAME"
else
    echo -e "${GREEN}✅ Bucket gs://$BUCKET_NAME exists${NC}"
fi

# Upload files
echo ""
echo "📤 Uploading website files..."
echo "   Source: ./website/"
echo "   Destination: gs://$BUCKET_NAME/"

# Set cache control for different file types
echo "📄 Uploading HTML files..."
gsutil -m -h "Cache-Control:public, max-age=300" cp -r "../website/*.html" "gs://$BUCKET_NAME/" 2>/dev/null || true

echo "🎨 Uploading CSS files..."
gsutil -m -h "Cache-Control:public, max-age=86400" cp -r "../website/*.css" "gs://$BUCKET_NAME/" 2>/dev/null || true

echo "📋 Uploading any other web assets..."
gsutil -m cp -r ../website/* "gs://$BUCKET_NAME/" 2>/dev/null || true

# Upload .spray configuration directory (includes redirects)  
if [ -d "../website/.spray" ]; then
    echo "⚙️  Uploading Spray configuration (.spray directory)..."
    gsutil -m cp -r "../website/.spray" "gs://$BUCKET_NAME/" 2>/dev/null || true
else
    echo "ℹ️  No .spray directory found - skipping configuration upload"
fi

echo ""
echo "🔍 Verifying upload..."
gsutil ls "gs://$BUCKET_NAME/"

echo ""
echo -e "${GREEN}✅ Deployment completed successfully!${NC}"
echo ""
echo "🌐 Your website files are now available in gs://$BUCKET_NAME"
echo ""
echo "🚀 To serve with Spray:"
echo "   docker run -e BUCKET_NAME=$BUCKET_NAME -e GOOGLE_PROJECT_ID=$PROJECT_ID -p 8080:8080 spray"
echo ""
echo "📊 Monitor your deployment (when Spray is running):"
echo "   Health: http://localhost:8080/livez"
echo "   Metrics: http://localhost:8080/metrics"
echo ""
echo "💡 To make changes:"
echo "   1. Edit files in the website/ directory"
echo "   2. Run this script again to deploy"
echo "" 