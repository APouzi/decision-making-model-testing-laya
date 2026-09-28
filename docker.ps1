param([switch]$Cpu, [int]$Port = 8765)
$ErrorActionPreference = 'Stop'
$layaRuntime = if ($env:LAYA_RUNTIME_DIR) { $env:LAYA_RUNTIME_DIR } else { Join-Path $env:LOCALAPPDATA 'laya-decision-demo' }
$env:LAYA_MODELS_DIR = if ($env:LAYA_MODELS_DIR) { $env:LAYA_MODELS_DIR } else { Join-Path $layaRuntime 'models' }
foreach ($model in @('laya', 'laya-typed-decisions')) {
    if (-not (Test-Path -LiteralPath (Join-Path $env:LAYA_MODELS_DIR "$model\model.safetensors"))) { throw 'Download the weights with setup.ps1 first, or use the fresh Docker setup in README.md.' }
}
$env:LAYA_PORT = [string]$Port
$composeArgs = @('compose', '-f', (Join-Path $PSScriptRoot 'compose.yaml'))
if (-not $Cpu) { $composeArgs += @('-f', (Join-Path $PSScriptRoot 'compose.gpu.yaml')) }
$composeArgs += @('-f', (Join-Path $PSScriptRoot 'compose.local-models.yaml'), 'up', '--build', '-d', '--wait', '--wait-timeout', '240')
Push-Location $PSScriptRoot
try {
    & docker @composeArgs
    if ($LASTEXITCODE -ne 0) { throw 'Docker startup failed. Inspect docker compose logs.' }
} finally { Pop-Location }
Write-Host "Svelte + Go + Laya: http://127.0.0.1:$Port"
