# HTTP Cache Performance Testing with k6
# This script runs comprehensive performance tests for HTTP cache implementation

param(
    [Parameter(Mandatory=$false)]
    [ValidateSet("quick", "comprehensive", "load")]
    [string]$TestType = "quick",
    
    [Parameter(Mandatory=$false)]
    [ValidateSet("local", "staging", "production")]
    [string]$Environment = "local",
    
    [Parameter(Mandatory=$false)]
    [string]$SprayUrl = "",
    
    [Parameter(Mandatory=$false)]
    [ValidateSet("console", "json", "influxdb", "prometheus", "cloud")]
    [string]$Output = "console",
    
    [Parameter(Mandatory=$false)]
    [switch]$InstallK6,
    
    [Parameter(Mandatory=$false)]
    [switch]$Verbose
)

# Colors for output
$Red = [System.ConsoleColor]::Red
$Green = [System.ConsoleColor]::Green
$Yellow = [System.ConsoleColor]::Yellow
$Blue = [System.ConsoleColor]::Blue
$White = [System.ConsoleColor]::White

function Write-ColorOutput($ForegroundColor, $Message) {
    $originalColor = [Console]::ForegroundColor
    [Console]::ForegroundColor = $ForegroundColor
    Write-Output $Message
    [Console]::ForegroundColor = $originalColor
}

function Test-K6Installation {
    try {
        $k6Version = k6 version 2>$null
        if ($k6Version) {
            Write-ColorOutput $Green "✅ k6 is installed: $($k6Version.Split("`n")[0])"
            return $true
        }
    }
    catch {
        Write-ColorOutput $Red "❌ k6 is not installed or not in PATH"
        return $false
    }
    return $false
}

function Install-K6 {
    Write-ColorOutput $Yellow "📦 Installing k6..."
    
    # Check if Chocolatey is available
    try {
        choco --version 2>$null | Out-Null
        Write-ColorOutput $Blue "Installing k6 via Chocolatey..."
        choco install k6 -y
        return $true
    }
    catch {
        Write-ColorOutput $Yellow "Chocolatey not found, trying winget..."
    }
    
    # Try winget
    try {
        winget --version 2>$null | Out-Null
        Write-ColorOutput $Blue "Installing k6 via winget..."
        winget install k6 --source winget
        return $true
    }
    catch {
        Write-ColorOutput $Red "❌ Neither Chocolatey nor winget available"
        Write-ColorOutput $White "Please install k6 manually from: https://k6.io/docs/get-started/installation/"
        return $false
    }
}

function Get-TestConfiguration($TestType) {
    $config = @{
        "quick" = @{
            Description = "Quick cache validation test - 2 minutes"
            Duration = "2 minutes"
            VUs = "5-10"
            Scenarios = @("baseline", "cache_validation")
        }
        "comprehensive" = @{
            Description = "Full cache performance test - 18 minutes"
            Duration = "18 minutes" 
            VUs = "10-100"
            Scenarios = @("baseline", "etag_cache", "lastmod_cache", "mixed_content", "stress_test")
        }
        "load" = @{
            Description = "High load test for production validation"
            Duration = "20 minutes"
            VUs = "50-300"
            Scenarios = @("high_load_cache")
        }
    }
    
    return $config[$TestType]
}

function Get-EnvironmentUrl($Environment, $CustomUrl) {
    if ($CustomUrl) {
        return $CustomUrl
    }
    
    $urls = @{
        "local" = "http://localhost:8080"
        "staging" = "https://spray-staging.example.com"
        "production" = "https://spray.example.com"
    }
    
    return $urls[$Environment]
}

function Test-SprayServer($Url) {
    Write-ColorOutput $Blue "🔍 Testing Spray server connectivity..."
    
    try {
        $response = Invoke-WebRequest -Uri "$Url/readyz" -Method GET -TimeoutSec 10
        if ($response.StatusCode -eq 200) {
            Write-ColorOutput $Green "✅ Spray server is accessible at $Url"
            return $true
        }
    }
    catch {
        Write-ColorOutput $Red "❌ Cannot reach Spray server at $Url"
        Write-ColorOutput $White "Error: $($_.Exception.Message)"
        return $false
    }
    
    return $false
}

function Get-OutputOptions($OutputType) {
    switch ($OutputType) {
        "console" { return @() }
        "json" { return @("--out", "json=k6-results-$(Get-Date -Format 'yyyy-MM-dd-HHmm').json") }
        "influxdb" { return @("--out", "influxdb=http://localhost:8086/k6") }
        "prometheus" { return @("--out", "experimental-prometheus-rw") }
        "cloud" { return @("--out", "cloud") }
        default { return @() }
    }
}

function Start-K6Test($TestType, $Url, $OutputOptions, $Verbose) {
    $testScript = "scripts/cache-performance-test.js"
    
    if (!(Test-Path $testScript)) {
        Write-ColorOutput $Red "❌ Test script not found: $testScript"
        return $false
    }
    
    Write-ColorOutput $Blue "🚀 Starting k6 cache performance test..."
    Write-ColorOutput $White "Test Type: $TestType"
    Write-ColorOutput $White "Target URL: $Url"
    Write-ColorOutput $White "Output: $($OutputOptions -join ' ')"
    Write-ColorOutput $White ""
    
    # Build k6 command
    $k6Args = @("run", $testScript)
    
    # Add environment variable
    $env:SPRAY_URL = $Url
    
    # Add output options
    if ($OutputOptions.Count -gt 0) {
        $k6Args += $OutputOptions
    }
    
    # Add verbose flag
    if ($Verbose) {
        $k6Args += "--verbose"
    }
    
    # Run the test
    try {
        Write-ColorOutput $Green "Executing: k6 $($k6Args -join ' ')"
        Write-ColorOutput $White "----------------------------------------"
        
        & k6 @k6Args
        
        $exitCode = $LASTEXITCODE
        if ($exitCode -eq 0) {
            Write-ColorOutput $Green "✅ k6 test completed successfully!"
            return $true
        } else {
            Write-ColorOutput $Red "❌ k6 test failed with exit code: $exitCode"
            return $false
        }
    }
    catch {
        Write-ColorOutput $Red "❌ Error running k6 test: $($_.Exception.Message)"
        return $false
    }
}

function Show-TestSummary($TestType, $Url, $Duration) {
    Write-ColorOutput $Blue "📊 Test Summary"
    Write-ColorOutput $Blue "==============="
    Write-ColorOutput $White "Test Type: $TestType"
    Write-ColorOutput $White "Target URL: $Url"
    Write-ColorOutput $White "Duration: $Duration"
    Write-ColorOutput $White ""
    
    Write-ColorOutput $Yellow "Key Metrics to Monitor:"
    Write-ColorOutput $White "• Cache Hit Rate (target: >80%)"
    Write-ColorOutput $White "• Response Time for 304s (target: <100ms)"
    Write-ColorOutput $White "• Response Time for 200s (baseline)"
    Write-ColorOutput $White "• Bandwidth Saved (bytes)"
    Write-ColorOutput $White "• Error Rate (target: <1%)"
    Write-ColorOutput $White ""
    
    Write-ColorOutput $Yellow "Prometheus Metrics to Check:"
    Write-ColorOutput $White "• gcs_server_cache_total{status=\"hit\"}"
    Write-ColorOutput $White "• gcs_server_cache_total{status=\"miss\"}"
    Write-ColorOutput $White "• gcs_server_requests_total{status=\"304\"}"
    Write-ColorOutput $White "• gcs_server_storage_operations_skipped_total"
    Write-ColorOutput $White ""
}

# Main execution
Write-ColorOutput $Blue "🚀 HTTP Cache Performance Testing with k6"
Write-ColorOutput $Blue "=========================================="
Write-ColorOutput $White ""

# Install k6 if requested
if ($InstallK6) {
    if (!(Install-K6)) {
        exit 1
    }
}

# Check k6 installation
if (!(Test-K6Installation)) {
    Write-ColorOutput $Yellow "Would you like to install k6? (Use -InstallK6 flag)"
    exit 1
}

# Get test configuration
$config = Get-TestConfiguration $TestType
Write-ColorOutput $White "Test Configuration:"
Write-ColorOutput $White "• Description: $($config.Description)"
Write-ColorOutput $White "• Duration: $($config.Duration)"
Write-ColorOutput $White "• Virtual Users: $($config.VUs)"
Write-ColorOutput $White "• Scenarios: $($config.Scenarios -join ', ')"
Write-ColorOutput $White ""

# Get target URL
$targetUrl = Get-EnvironmentUrl $Environment $SprayUrl
Write-ColorOutput $White "Target URL: $targetUrl"

# Test server connectivity
if (!(Test-SprayServer $targetUrl)) {
    Write-ColorOutput $Red "Cannot proceed with tests - server not accessible"
    exit 1
}

# Get output options
$outputOptions = Get-OutputOptions $Output

# Show test summary
Show-TestSummary $TestType $targetUrl $config.Duration

# Confirm before running
Write-ColorOutput $Yellow "Ready to start the test. Press any key to continue or Ctrl+C to cancel..."
$null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
Write-ColorOutput $White ""

# Run the test
$success = Start-K6Test $TestType $targetUrl $outputOptions $Verbose

if ($success) {
    Write-ColorOutput $Green "🎉 Cache performance test completed successfully!"
    Write-ColorOutput $White ""
    Write-ColorOutput $Yellow "Next Steps:"
    Write-ColorOutput $White "1. Review the k6 output above for performance metrics"
    Write-ColorOutput $White "2. Check your Prometheus metrics at $targetUrl/metrics"
    Write-ColorOutput $White "3. Compare cache hit rates and response times"
    Write-ColorOutput $White "4. Monitor bandwidth savings and GCS operations saved"
    
    if ($Output -eq "json") {
        Write-ColorOutput $White "5. Analyze detailed results in the generated JSON file"
    }
} else {
    Write-ColorOutput $Red "❌ Test failed. Check the output above for details."
    exit 1
}

Write-ColorOutput $White ""
Write-ColorOutput $Blue "For more detailed analysis, use the Prometheus queries in:"
Write-ColorOutput $Blue "scripts/prometheus-queries.md" 