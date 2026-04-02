# deploy.ps1 — Windows-native deploy script for Samverk
# Usage: .\scripts\deploy.ps1 [-Host 192.168.1.162] [-User root] [-RebuildWeb]
param(
    [string]$DeployHost = "192.168.1.162",
    [string]$DeployUser = "root",
    [switch]$RebuildWeb
)

$ErrorActionPreference = "Stop"

$BIN = "samverk"
$VERSION_PKG = "samverk.dev/samverk/internal/version"
$VERSION = (git describe --tags --always --dirty 2>$null) ?? "dev"
$COMMIT = (git rev-parse --short HEAD 2>$null) ?? "unknown"
$DATE = (Get-Date -Format "yyyy-MM-ddTHH:mm:ssZ")
$LDFLAGS = "-s -w -X ${VERSION_PKG}.Version=${VERSION} -X ${VERSION_PKG}.GitCommit=${COMMIT} -X ${VERSION_PKG}.BuildDate=${DATE}"

# Step 1: Optionally rebuild SPA
if ($RebuildWeb) {
    Write-Host "Building SPA..." -ForegroundColor Cyan
    Push-Location web
    try {
        npx pnpm install --no-frozen-lockfile
        npx pnpm build
    } finally {
        Pop-Location
    }
    Copy-Item -Recurse -Force "web\dist\*" "internal\server\static\"
}

# Step 2: Cross-compile for Linux
Write-Host "Cross-compiling for Linux amd64..." -ForegroundColor Cyan
$env:GOOS = "linux"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"
try {
    go build -ldflags $LDFLAGS -o "bin\${BIN}-linux-amd64" ./cmd/samverk/
} finally {
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
}
Write-Host "Built bin\${BIN}-linux-amd64" -ForegroundColor Green

# Step 3: Stop services on remote host
Write-Host "Stopping services on ${DeployHost}..." -ForegroundColor Cyan
ssh "${DeployUser}@${DeployHost}" "systemctl stop samverk-dispatch samverk-serve 2>/dev/null || true"

# Step 4: Copy binary
Write-Host "Deploying binary..." -ForegroundColor Cyan
scp "bin\${BIN}-linux-amd64" "${DeployUser}@${DeployHost}:/usr/local/bin/${BIN}"

# Step 5: Copy config and restart
Write-Host "Deploying config and restarting..." -ForegroundColor Cyan
scp deploy/samverk-serve.service deploy/samverk-dispatch.service "${DeployUser}@${DeployHost}:/tmp/"
scp deploy/install.sh "${DeployUser}@${DeployHost}:/tmp/install.sh"
ssh "${DeployUser}@${DeployHost}" "sed -i 's/\r$//' /tmp/install.sh && bash /tmp/install.sh && systemctl start samverk-serve samverk-dispatch"

# Step 6: Health check
Write-Host "Verifying health..." -ForegroundColor Cyan
Start-Sleep -Seconds 3
$health = ssh "${DeployUser}@${DeployHost}" "curl -sf http://localhost:8080/healthz"
if ($LASTEXITCODE -eq 0) {
    Write-Host " OK" -ForegroundColor Green
} else {
    Write-Host " FAIL" -ForegroundColor Red
    exit 1
}
