$ErrorActionPreference = 'Stop'
$layaRuntime = if ($env:LAYA_RUNTIME_DIR) { $env:LAYA_RUNTIME_DIR } else { Join-Path $env:LOCALAPPDATA 'laya-decision-demo' }
foreach ($tool in @('node', 'npm.cmd', 'go')) {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) { throw "Install $tool before building (Node 20+, Go 1.25+)." }
}
$layaBuild = Join-Path $layaRuntime 'build'
$layaBin = Join-Path $layaRuntime 'bin'
New-Item -ItemType Directory -Path $layaBuild, $layaBin -Force | Out-Null
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'frontend') -Destination $layaBuild -Recurse -Force
Push-Location (Join-Path $layaBuild 'frontend')
try {
    & npm.cmd ci --no-fund
    if ($LASTEXITCODE -ne 0) { throw 'Frontend dependency installation failed.' }
    & npm.cmd run check
    if ($LASTEXITCODE -ne 0) { throw 'Svelte validation failed.' }
    & npm.cmd run build
    if ($LASTEXITCODE -ne 0) { throw 'Svelte build failed.' }
} finally { Pop-Location }
$layaGo = Join-Path $layaBuild 'backend'
New-Item -ItemType Directory -Path (Join-Path $layaGo 'web') -Force | Out-Null
Get-ChildItem -LiteralPath (Join-Path $PSScriptRoot 'backend') -File | ForEach-Object {
    Copy-Item -LiteralPath $_.FullName -Destination $layaGo -Force
}
Get-ChildItem -LiteralPath (Join-Path $layaBuild 'frontend\dist') | ForEach-Object {
    Copy-Item -LiteralPath $_.FullName -Destination (Join-Path $layaGo 'web') -Recurse -Force
}
$previousCGO = $env:CGO_ENABLED
$env:CGO_ENABLED = '0'
Push-Location $layaGo
try {
    & go build -trimpath -ldflags '-s -w' -o (Join-Path $layaBin 'laya-demo.exe') .
    if ($LASTEXITCODE -ne 0) { throw 'Go build failed.' }
} finally { Pop-Location; $env:CGO_ENABLED = $previousCGO }
Write-Host "Built Svelte and Go: $(Join-Path $layaBin 'laya-demo.exe')"
