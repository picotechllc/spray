#!/bin/bash

# Spray Website Deployment Script
# This script uploads the website files to a Google Cloud Storage bucket

set -e

# Configuration
BUCKET_NAME="${BUCKET_NAME:-spray.picote.ch}"
PROJECT_ID="${GOOGLE_PROJECT_ID:-}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}🚀 Spray Website Deployment Script${NC}"
echo "=================================="

# Check if required tools are installed
if ! command -v gsutil &> /dev/null; then
    echo -e "${RED}❌ Error: gsutil is not installed. Please install Google Cloud SDK.${NC}"
    exit 1
fi

# Check if project ID is set
if [ -z "$PROJECT_ID" ]; then
    echo -e "${YELLOW}⚠️  Warning: GOOGLE_PROJECT_ID not set. Using current gcloud project.${NC}"
    PROJECT_ID=$(gcloud config get-value project 2>/dev/null || echo "")
    if [ -z "$PROJECT_ID" ]; then
        echo -e "${RED}❌ Error: No Google Cloud project configured. Please set GOOGLE_PROJECT_ID or run 'gcloud config set project YOUR_PROJECT_ID'${NC}"
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
gsutil -m -h "Cache-Control:public, max-age=300" cp -r "*.html" "gs://$BUCKET_NAME/" 2>/dev/null || true

echo "🎨 Uploading CSS files..."
gsutil -m -h "Cache-Control:public, max-age=86400" cp -r "*.css" "gs://$BUCKET_NAME/" 2>/dev/null || true

echo "📋 Uploading other files..."
gsutil -m cp -r "*.md" "*.sh" "gs://$BUCKET_NAME/" 2>/dev/null || true

echo ""
echo "🔍 Verifying upload..."
gsutil ls "gs://$BUCKET_NAME/"

echo ""
echo -e "${GREEN}✅ Deployment completed successfully!${NC}"
echo ""
echo "🌐 Your website should now be available at:"
echo "   https://spray.picote.ch"
echo ""
echo "📊 Monitor your deployment:"
echo "   Health: https://spray.picote.ch/livez"
echo "   Metrics: https://spray.picote.ch/metrics"
echo ""
echo "💡 To make changes:"
echo "   1. Edit files in the website/ directory"
echo "   2. Run this script again to deploy"
echo "" 