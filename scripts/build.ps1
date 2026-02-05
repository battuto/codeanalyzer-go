# Build cross-platform binaries for codeanalyzer-go (64-bit only)
# Usage: .\scripts\build.ps1

$ErrorActionPreference = "Stop"

$targets = @(
    @{GOOS="windows"; GOARCH="amd64"; Name="codeanalyzer-go-windows-amd64.exe"},
    @{GOOS="linux";   GOARCH="amd64"; Name="codeanalyzer-go-linux-amd64"},
    @{GOOS="darwin";  GOARCH="amd64"; Name="codeanalyzer-go-darwin-amd64"},
    @{GOOS="darwin";  GOARCH="arm64"; Name="codeanalyzer-go-darwin-arm64"}
)

# Ensure bin directory exists
$binDir = Join-Path $PSScriptRoot "..\bin"
if (-not (Test-Path $binDir)) {
    New-Item -ItemType Directory -Path $binDir | Out-Null
}

Write-Host "Building codeanalyzer-go for all platforms..." -ForegroundColor Cyan

foreach ($t in $targets) {
    $env:GOOS = $t.GOOS
    $env:GOARCH = $t.GOARCH
    $outPath = Join-Path $binDir $t.Name
    
    Write-Host "  Building $($t.Name)..." -ForegroundColor Yellow
    
    go build -ldflags "-s -w" -o $outPath ./cmd/codeanalyzer-go
    
    if ($LASTEXITCODE -eq 0) {
        $size = (Get-Item $outPath).Length / 1MB
        Write-Host "    OK ($([math]::Round($size, 2)) MB)" -ForegroundColor Green
    } else {
        Write-Host "    FAILED" -ForegroundColor Red
        exit 1
    }
}

# Reset environment
Remove-Item Env:GOOS -ErrorAction SilentlyContinue
Remove-Item Env:GOARCH -ErrorAction SilentlyContinue

Write-Host "`nAll builds completed successfully!" -ForegroundColor Green
Write-Host "Binaries are in: $binDir" -ForegroundColor Cyan
