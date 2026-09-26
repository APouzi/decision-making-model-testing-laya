$ErrorActionPreference = 'Stop'
$layaRuntime = if ($env:LAYA_RUNTIME_DIR) { $env:LAYA_RUNTIME_DIR } else { Join-Path $env:LOCALAPPDATA 'laya-decision-demo' }
if (-not (Get-Command uv -ErrorAction SilentlyContinue)) {
    throw 'Install uv first: https://docs.astral.sh/uv/getting-started/installation/'
}
New-Item -ItemType Directory -Path $layaRuntime -Force | Out-Null
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'pyproject.toml') -Destination (Join-Path $layaRuntime 'pyproject.toml')
$env:UV_CACHE_DIR = Join-Path $layaRuntime 'cache\uv'
$env:PYTHONDONTWRITEBYTECODE = '1'
$pythonPath = (Get-Command python -ErrorAction Stop).Source
uv sync --project $layaRuntime --python $pythonPath
if ($LASTEXITCODE -ne 0) { throw 'Dependency installation failed.' }
& (Join-Path $layaRuntime '.venv\Scripts\python.exe') -B (Join-Path $PSScriptRoot 'download_model.py')
if ($LASTEXITCODE -ne 0) { throw 'Model download failed.' }
& (Join-Path $PSScriptRoot 'build.ps1')
Write-Host 'Ready. Run .\start.ps1 and open http://127.0.0.1:8765'
