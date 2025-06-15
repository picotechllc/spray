# Spray Website Deployment Script (PowerShell)
# This script uploads the website files to a Google Cloud Storage bucket

param(
    [string]$BucketName = $env:BUCKET_NAME,
    [string]$ProjectId = $env:GOOGLE_PROJECT_ID
)

# Set default bucket name if not provided
if (-not $BucketName) {
    $BucketName = "spray.picote.ch"
}

Write-Host "🚀 Spray Website Deployment Script" -ForegroundColor Green
Write-Host "=================================="

# Check if gsutil is available
if (-not (Get-Command gsutil -ErrorAction SilentlyContinue)) {
    Write-Host "❌ Error: gsutil is not installed. Please install Google Cloud SDK." -ForegroundColor Red
    exit 1
}

# Check if project ID is set
if (-not $ProjectId) {
    Write-Host "⚠️  Warning: GOOGLE_PROJECT_ID not set. Using current gcloud project." -ForegroundColor Yellow
    $ProjectId = (gcloud config get-value project 2>$null)
    if (-not $ProjectId) {
        Write-Host "❌ Error: No Google Cloud project configured. Please set GOOGLE_PROJECT_ID or run 'gcloud config set project YOUR_PROJECT_ID'" -ForegroundColor Red
        exit 1
    }
}

Write-Host "📋 Configuration:"
Write-Host "   Bucket: $BucketName"
Write-Host "   Project: $ProjectId"
Write-Host ""

# Check if bucket exists
Write-Host "🔍 Checking if bucket exists..."
$bucketExists = $false
try {
    gsutil ls -b "gs://$BucketName" 2>$null | Out-Null
    $bucketExists = $true
} catch {
    $bucketExists = $false
}

if (-not $bucketExists) {
    Write-Host "📦 Bucket doesn't exist. Creating gs://$BucketName..." -ForegroundColor Yellow
    gsutil mb -p $ProjectId "gs://$BucketName"
    
    Write-Host "🔓 Making bucket publicly readable..."
    gsutil iam ch allUsers:objectViewer "gs://$BucketName"
} else {
    Write-Host "✅ Bucket gs://$BucketName exists" -ForegroundColor Green
}

# Change to website directory
if (Test-Path "website") {
    Set-Location "website"
}

# Upload files
Write-Host ""
Write-Host "📤 Uploading website files..."
Write-Host "   Source: ./website/"
Write-Host "   Destination: gs://$BucketName/"

# Upload HTML files with short cache
Write-Host "📄 Uploading HTML files..."
if (Test-Path "*.html") {
    gsutil -m -h "Cache-Control:public, max-age=300" cp *.html "gs://$BucketName/"
}

# Upload CSS files with longer cache
Write-Host "🎨 Uploading CSS files..."
if (Test-Path "*.css") {
    gsutil -m -h "Cache-Control:public, max-age=86400" cp *.css "gs://$BucketName/"
}

# Upload other files
Write-Host "📋 Uploading other files..."
if (Test-Path "*.md") {
    gsutil -m cp *.md "gs://$BucketName/"
}

Write-Host ""
Write-Host "🔍 Verifying upload..."
gsutil ls "gs://$BucketName/"

Write-Host ""
Write-Host "✅ Deployment completed successfully!" -ForegroundColor Green
Write-Host ""
Write-Host "🌐 Your website should now be available at:"
Write-Host "   https://spray.picote.ch"
Write-Host ""
Write-Host "📊 Monitor your deployment:"
Write-Host "   Health: https://spray.picote.ch/livez"
Write-Host "   Metrics: https://spray.picote.ch/metrics"
Write-Host ""
Write-Host "💡 To make changes:"
Write-Host "   1. Edit files in the website/ directory"
Write-Host "   2. Run this script again to deploy"
Write-Host "" 