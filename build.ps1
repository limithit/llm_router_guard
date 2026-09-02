# ====== AI Router Guard — 本地构建脚本 (PowerShell) ======
# Usage: .\build.ps1 [--clean] [--all]

param([switch]$Clean, [switch]$All)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

Write-Host "`n=== Building llm-router-guard ===`n" -ForegroundColor Cyan

# ---------- 前端 ----------
Write-Host "[1/2] Frontend..." -ForegroundColor Yellow
Set-Location frontend
if (-Not (Test-Path node_modules)) {
    Write-Host "  npm install (首次或清理后)" -ForegroundColor Gray
    npm install
}
npm run build
Set-Location ..

# ---------- 后端 (嵌入前端 dist) ----------
Write-Host "[2/2] Backend..." -ForegroundColor Yellow
Set-Location backend
if ($Clean) {
    Remove-Item -Recurse -Force web/dist -ErrorAction SilentlyContinue
}
New-Item -ItemType Directory -Path web/dist -Force | Out-Null
Copy-Item -Path "../frontend/dist/*" -Destination "web/dist/" -Recurse -Force

go build -ldflags="-s -w" -o ../bin/server ./cmd/server
Set-Location ..

New-Item -ItemType Directory -Path bin -Force | Out-Null
Write-Host "`n✓ Done — binary at bin/server" -ForegroundColor Green
Write-Host "`nRun:" -ForegroundColor White
Write-Host "  cd bin && ./server" -ForegroundColor Gray
Write-Host ""
