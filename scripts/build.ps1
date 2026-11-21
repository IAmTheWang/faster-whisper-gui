<#
.SYNOPSIS
  Full verified build, mirroring .github/workflows/ci.yml: frontend (install +
  tests + type-check + bundle) first, then Go (gofmt, vet, tests, and the
  single-exe server build).

.DESCRIPTION
  `go test -race` needs cgo; when no C compiler (gcc) is on PATH the script
  says so and runs the tests without it (CI always runs with -race).

.PARAMETER Restore
  After building, restore the tracked placeholder internal/webui/dist/index.html
  that the frontend build overwrites, so `git status` stays clean. (A later
  plain `go build` would then embed the placeholder again - re-run this script.)
#>
param([switch]$Restore)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

foreach ($tool in 'go', 'bun') {
  if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
    throw "'$tool' was not found on PATH - install it first (see README.md)."
  }
}

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

Step 'gofmt' {
  $unformatted = gofmt -l .
  if ($unformatted) {
    Write-Host "Not gofmt-formatted:`n$unformatted" -ForegroundColor Red
    $global:LASTEXITCODE = 1
  } else {
    $global:LASTEXITCODE = 0
  }
}
Step 'go mod tidy' { go mod tidy -diff }
Step 'go vet' { go vet ./... }

if (Get-Command gcc -ErrorAction SilentlyContinue) {
  $env:CGO_ENABLED = '1'
  Step 'go test -race' { go test -race ./... }
} else {
  Write-Warning 'gcc not found: running go test WITHOUT -race (CI does use it).'
  Step 'go test' { go test ./... }
}

# Version: the nearest tag plus the commit (and "-dirty"), or "dev" outside git.
$version = (git describe --tags --always --dirty 2>$null)
if (-not $version) { $version = 'dev' }
Step "go build ($version)" {
  go build -trimpath -ldflags "-s -w -X main.version=$version" -o faster-whisper-gui.exe ./cmd/server
}

if ($Restore) {
  git checkout -- internal/webui/dist/index.html
}
Write-Host "Built $root\faster-whisper-gui.exe ($version)" -ForegroundColor Green
