$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$python = if ($env:CODEX_PYTHON) { $env:CODEX_PYTHON } else { 'python' }
& $python (Join-Path $PSScriptRoot 'render_whitepaper.py')
$tmp = Join-Path $root 'tmp/pdfs'
New-Item -ItemType Directory -Force $tmp | Out-Null
pdftoppm -png -r 144 (Join-Path $root 'output/pdf/typeference-whitepaper.pdf') (Join-Path $tmp 'typeference-page')
pdfinfo (Join-Path $root 'output/pdf/typeference-whitepaper.pdf')
