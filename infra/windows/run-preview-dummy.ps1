param([int] $Port = 8765)

$ErrorActionPreference = 'Stop'
$ProjectRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$python = Join-Path $ProjectRoot '.venv\Scripts\python.exe'
if (-not (Test-Path -LiteralPath $python -PathType Leaf)) { throw 'Buat .venv dan pasang apps/core-api/requirements.txt terlebih dahulu.' }

# Isolated preview settings. Process environment takes precedence over .env.local.
$env:APP_ENV = 'development'
$env:PREVIEW_DUMMY_DATA = '1'
$env:PREVIEW_RESET = '1'
$env:DATABASE_URL = 'sqlite:///D:/brutalx/python/algentraCapitalProject/.preview-dashboard.sqlite3'
$env:APP_SECRET_KEY = 'local-dashboard-preview-only-secret-key-32-characters'
$env:ADMIN_USERNAME = 'preview-admin'
$env:ADMIN_PASSWORD = 'preview-only-password-1234'
$env:COOKIE_SECURE = 'false'
$env:GEMINI_API_KEY = ''
$env:GEMINI_CS_API_KEY = ''
$env:TELEGRAM_API_ID = ''
$env:TELEGRAM_API_HASH = ''
$env:TELEGRAM_BOT_TOKEN = ''
$env:TELEGRAM_ADMIN_CHAT_ID = ''
$env:EA_API_KEY = ''
$env:COPIER_WORKER_API_KEY = ''
$env:BFF_HMAC_SECRET = ''
$env:EMAIL_SMTP_HOST = ''
$env:EMAIL_SMTP_USERNAME = ''
$env:EMAIL_SMTP_PASSWORD = ''
$env:EMAIL_FROM = ''
$env:WEBSOCKET_ALLOWED_ORIGINS = ''

Set-Location -LiteralPath $ProjectRoot
& $python -c "import sys; sys.path.insert(0, 'apps/core-api'); import app.models; from app.database import Base, engine, migrate_schema; migrate_schema(); Base.metadata.create_all(engine); from app.seed_preview_data import main; main()"
if ($LASTEXITCODE -ne 0) { throw 'Gagal menyiapkan data dummy dashboard.' }
$env:PREVIEW_RESET = '0'
& $python -m uvicorn app.main:app --app-dir apps/core-api --host 127.0.0.1 --port $Port
