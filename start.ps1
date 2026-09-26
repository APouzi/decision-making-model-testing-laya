param([int]$Port = 8765)
$ErrorActionPreference = 'Stop'
$layaRuntime = if ($env:LAYA_RUNTIME_DIR) { $env:LAYA_RUNTIME_DIR } else { Join-Path $env:LOCALAPPDATA 'laya-decision-demo' }
$layaPython = Join-Path $layaRuntime '.venv\Scripts\python.exe'
$layaServer = Join-Path $layaRuntime 'bin\laya-demo.exe'
if (-not (Test-Path -LiteralPath $layaPython)) { throw 'Run .\setup.ps1 first.' }
if (-not (Test-Path -LiteralPath $layaServer)) { & (Join-Path $PSScriptRoot 'build.ps1') }
$env:PYTHONDONTWRITEBYTECODE = '1'
Write-Host "Laya playground: http://127.0.0.1:$Port (Ctrl+C to stop)"
& $layaServer --source-dir $PSScriptRoot --python $layaPython --port $Port
