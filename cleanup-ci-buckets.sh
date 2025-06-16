#!/bin/bash
set -e

# Configuration
PROJECT_ID="${PROJECT_ID:-your-project-id}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}Cleaning up GCS buckets for Spray CI...${NC}"

# Function to check if bucket exists
bucket_exists() {
    gsutil ls -b "gs://$1" >/dev/null 2>&1
}

# Function to delete bucket and all contents
delete_bucket() {
    local bucket_name="$1"
    
    if bucket_exists "$bucket_name"; then
        echo -e "${YELLOW}Deleting bucket gs://$bucket_name and all contents...${NC}"
        gsutil -m rm -r "gs://$bucket_name"
        echo -e "${GREEN}✓ Deleted bucket gs://$bucket_name${NC}"
    else
        echo -e "${YELLOW}Bucket gs://$bucket_name doesn't exist (already deleted?)${NC}"
    fi
}

# Validate required variables
if [ "$PROJECT_ID" = "your-project-id" ]; then
    echo -e "${RED}Error: Please set PROJECT_ID environment variable${NC}"
    echo "Example: export PROJECT_ID=my-gcp-project"
    exit 1
fi

echo -e "${YELLOW}Configuration:${NC}"
echo "  Project ID: $PROJECT_ID"
echo ""

# List of buckets to clean up
BUCKETS=(
    "spray-test-bucket-TestGCSIntegration"
)

# Confirm deletion
echo -e "${RED}WARNING: This will permanently delete the following buckets and ALL their contents:${NC}"
for bucket in "${BUCKETS[@]}"; do
    echo "  - gs://$bucket"
done
echo ""

read -p "Are you sure you want to proceed? (y/N): " -n 1 -r
echo ""

if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo -e "${YELLOW}Cleanup cancelled.${NC}"
    exit 0
fi

# Delete buckets
for bucket in "${BUCKETS[@]}"; do
    delete_bucket "$bucket"
done

echo ""
echo -e "${GREEN}✓ Cleanup completed successfully!${NC}" 