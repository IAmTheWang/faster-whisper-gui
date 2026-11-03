<#
.SYNOPSIS
  Full verified build: frontend (tests + type-check + bundle) first, then Go
  (vet + tests + the single-exe server build).

.PARAMETER Restore
  After building, restore the tracked placeholder internal/webui/dist/index.html
  that the frontend build overwrites, so `git status` stays clean. (A later
  plain `go build` would then embed the placeholder again - re-run this script.)
#>
param([switch]$Restore)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

function Step($name, [scriptblock]$body) {
  Write-Host "==> $name" -ForegroundColor Cyan
  & $body
  if ($LASTEXITCODE -ne 0) { throw "$name failed (exit $LASTEXITCODE)" }
}

# Always `bun run <script>`: bare `bun build` is bun's own bundler, not package.json's script.
Push-Location web
try {
  Step 'bun install'   { bun install --frozen-lockfile }
  Step 'frontend test' { bun run test }
  Step 'frontend build (tsc + vite)' { bun run build }
} finally { Pop-Location }

Step 'go vet'   { go vet ./... }
Step 'go test'  { go test ./... }
Step 'go build' { go build -o faster-whisper-gui.exe ./cmd/server }

if ($Restore) {
  git checkout -- internal/webui/dist/index.html
}
Write-Host "Built $root\faster-whisper-gui.exe" -ForegroundColor Green
