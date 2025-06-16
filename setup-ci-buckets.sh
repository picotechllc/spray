#!/bin/bash
set -e

# Configuration
PROJECT_ID="${PROJECT_ID:-your-project-id}"
CI_SERVICE_ACCOUNT="${CI_SERVICE_ACCOUNT:-your-ci-service-account@your-project-id.iam.gserviceaccount.com}"
LOCATION="${LOCATION:-us-central1}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Setting up GCS buckets for Spray CI...${NC}"

# Function to check if bucket exists
bucket_exists() {
    gsutil ls -b "gs://$1" >/dev/null 2>&1
}

# Function to create bucket if it doesn't exist
create_bucket_if_not_exists() {
    local bucket_name="$1"
    local location="$2"
    
    if bucket_exists "$bucket_name"; then
        echo -e "${YELLOW}Bucket gs://$bucket_name already exists${NC}"
    else
        echo -e "${GREEN}Creating bucket gs://$bucket_name...${NC}"
        gsutil mb -p "$PROJECT_ID" -c STANDARD -l "$location" "gs://$bucket_name"
        echo -e "${GREEN}✓ Created bucket gs://$bucket_name${NC}"
    fi
}

# Function to set bucket permissions
set_bucket_permissions() {
    local bucket_name="$1"
    local service_account="$2"
    
    echo -e "${GREEN}Setting permissions for $service_account on gs://$bucket_name...${NC}"
    
    # Grant necessary permissions for CI testing
    gsutil iam ch "serviceAccount:$service_account:roles/storage.objectViewer" "gs://$bucket_name"
    gsutil iam ch "serviceAccount:$service_account:roles/storage.objectCreator" "gs://$bucket_name"
    
    echo -e "${GREEN}✓ Permissions set for gs://$bucket_name${NC}"
}

# Function to create test object
create_test_object() {
    local bucket_name="$1"
    
    echo -e "${GREEN}Creating test object in gs://$bucket_name...${NC}"
    echo "Hello, World!" | gsutil cp - "gs://$bucket_name/test.txt"
    gsutil setmeta -h "Content-Type:text/plain" "gs://$bucket_name/test.txt"
    echo -e "${GREEN}✓ Created test object gs://$bucket_name/test.txt${NC}"
}

# Validate required variables
if [ "$PROJECT_ID" = "your-project-id" ]; then
    echo -e "${RED}Error: Please set PROJECT_ID environment variable${NC}"
    echo "Example: export PROJECT_ID=my-gcp-project"
    exit 1
fi

if [ "$CI_SERVICE_ACCOUNT" = "your-ci-service-account@your-project-id.iam.gserviceaccount.com" ]; then
    echo -e "${RED}Error: Please set CI_SERVICE_ACCOUNT environment variable${NC}"
    echo "Example: export CI_SERVICE_ACCOUNT=github-actions@my-project.iam.gserviceaccount.com"
    exit 1
fi

echo -e "${YELLOW}Configuration:${NC}"
echo "  Project ID: $PROJECT_ID"
echo "  CI Service Account: $CI_SERVICE_ACCOUNT"
echo "  Location: $LOCATION"
echo ""

# List of buckets needed for CI
BUCKETS=(
    "spray-test-bucket-TestGCSIntegration"
)

# Create buckets and set permissions
for bucket in "${BUCKETS[@]}"; do
    echo -e "${GREEN}Processing bucket: $bucket${NC}"
    create_bucket_if_not_exists "$bucket" "$LOCATION"
    set_bucket_permissions "$bucket" "$CI_SERVICE_ACCOUNT"
    create_test_object "$bucket"
    echo ""
done

echo -e "${GREEN}✓ All CI buckets have been set up successfully!${NC}"
echo ""
echo -e "${YELLOW}Next steps:${NC}"
echo "1. Set TEST_BUCKET environment variable in your CI to use pre-existing bucket:"
echo "   export TEST_BUCKET=spray-test-bucket-TestGCSIntegration"
echo ""
echo "2. Verify permissions by running integration tests:"
echo "   go test -tags=integration ./..."
echo ""
echo -e "${GREEN}Buckets created:${NC}"
for bucket in "${BUCKETS[@]}"; do
    echo "  - gs://$bucket"
done 