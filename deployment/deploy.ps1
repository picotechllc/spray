# Static Website Deployment Script for Spray (PowerShell)
# This script uploads the website files to a Google Cloud Storage bucket

param(
    [string]$BucketName = $env:BUCKET_NAME,
    [string]$ProjectId = $env:GOOGLE_PROJECT_ID,
    [switch]$SprayProject
)

# Determine website directory relative to script location
$WebsiteDir = Join-Path (Split-Path $PSScriptRoot -Parent) "website"

# Use Spray project defaults if requested
if ($SprayProject) {
    if (-not $BucketName) { $BucketName = "spray.picote.ch" }
    if (-not $ProjectId) { $ProjectId = "shared-k8s-prd" }
    Write-Host "[INFO] Using Spray project defaults" -ForegroundColor Cyan
}

# Check if bucket name is provided
if (-not $BucketName) {
    Write-Host "[ERROR] BUCKET_NAME is required." -ForegroundColor Red
    Write-Host "Options:"
    Write-Host "  1. Set environment variable: `$env:BUCKET_NAME = 'your-bucket-name'"
    Write-Host "  2. Pass as parameter: ./deploy.ps1 -BucketName 'your-bucket-name'"
    Write-Host "  3. Use Spray project defaults: ./deploy.ps1 -SprayProject"
    exit 1
}

Write-Host "Static Website Deployment Script" -ForegroundColor Green
Write-Host "====================================="

# Check if gsutil is available
if (-not (Get-Command gsutil -ErrorAction SilentlyContinue)) {
    Write-Host "[ERROR] gsutil is not installed. Please install Google Cloud SDK." -ForegroundColor Red
    exit 1
}

# Check if project ID is set
if (-not $ProjectId) {
    if (-not $SprayProject) {
        Write-Host "[WARNING] GOOGLE_PROJECT_ID not set. Using current gcloud project." -ForegroundColor Yellow
    }
    $ProjectId = (gcloud config get-value project 2>$null)
    if (-not $ProjectId) {
        Write-Host "[ERROR] No Google Cloud project configured." -ForegroundColor Red
        Write-Host "Options:"
        Write-Host "  1. Set environment variable: `$env:GOOGLE_PROJECT_ID = 'your-project-id'"
        Write-Host "  2. Configure gcloud: gcloud config set project your-project-id"
        if (-not $SprayProject) {
            Write-Host "  3. Use Spray project defaults: ./deploy.ps1 -SprayProject"
        }
        exit 1
    }
}

Write-Host "Configuration:"
Write-Host "   Bucket: $BucketName"
Write-Host "   Project: $ProjectId"
Write-Host "   Website Directory: $WebsiteDir"
Write-Host ""

# Check if bucket exists
Write-Host "Checking if bucket exists..."
$bucketExists = $false
try {
    gsutil ls -b "gs://$BucketName" 2>$null | Out-Null
    $bucketExists = $true
} catch {
    $bucketExists = $false
}

if (-not $bucketExists) {
    Write-Host "Bucket doesn't exist. Creating gs://$BucketName..." -ForegroundColor Yellow
    gsutil mb -p $ProjectId "gs://$BucketName"
    
    Write-Host "Making bucket publicly readable..."
    gsutil iam ch allUsers:objectViewer "gs://$BucketName"
} else {
    Write-Host "Bucket gs://$BucketName exists" -ForegroundColor Green
}

# Check if website directory exists
if (-not (Test-Path $WebsiteDir)) {
    Write-Host "[ERROR] Website directory not found: $WebsiteDir" -ForegroundColor Red
    exit 1
}

# Upload files
Write-Host ""
Write-Host "Uploading website files..."
Write-Host "   Source: $WebsiteDir"
Write-Host "   Destination: gs://$BucketName/"

# Upload HTML files with short cache
Write-Host "Uploading HTML files..."
$htmlFiles = Join-Path $WebsiteDir "*.html"
if (Test-Path $htmlFiles) {
    gsutil -m -h "Cache-Control:public, max-age=300" cp $htmlFiles "gs://$BucketName/"
}

# Upload CSS files with longer cache
Write-Host "Uploading CSS files..."
$cssFiles = Join-Path $WebsiteDir "*.css"
if (Test-Path $cssFiles) {
    gsutil -m -h "Cache-Control:public, max-age=86400" cp $cssFiles "gs://$BucketName/"
}

# Upload any other web assets
Write-Host "Uploading any other web assets..."
$allFiles = Join-Path $WebsiteDir "*"
gsutil -m cp -r $allFiles "gs://$BucketName/"

Write-Host ""
Write-Host "Verifying upload..."
gsutil ls "gs://$BucketName/"

Write-Host ""
Write-Host "Deployment completed successfully!" -ForegroundColor Green
Write-Host ""
Write-Host "Your website files are now available in gs://$BucketName"
Write-Host ""
Write-Host "To serve with Spray:"
Write-Host "   docker run -e BUCKET_NAME=$BucketName -e GOOGLE_PROJECT_ID=$ProjectId -p 8080:8080 spray"
Write-Host ""
Write-Host "Monitor your deployment (when Spray is running):"
Write-Host "   Health: http://localhost:8080/livez"
Write-Host "   Metrics: http://localhost:8080/metrics"
Write-Host ""
Write-Host "To make changes:"
Write-Host "   1. Edit files in the website/ directory"
Write-Host "   2. Run this script again to deploy"