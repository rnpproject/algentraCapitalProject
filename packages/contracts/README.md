# API contracts

`openapi.json` is the checked-in contract for the versioned API. The existing
`/api/...` endpoints remain available for installed clients while consumers move
to `/v1/...` paths. Dashboard operations are under `/v1/dashboard`; public
performance endpoints are under `/v1/public`.

Regenerate the schema from the FastAPI application with the development/test
environment selected:

```powershell
$env:APP_ENV = 'test'
$env:APP_SECRET_KEY = 'm1-test-secret-key-at-least-thirty-two-characters'
$env:ADMIN_USERNAME = 'admin'
$env:ADMIN_PASSWORD = 'M1-Testing-Password-123'
$env:EA_API_KEY = 'm4-test-ea-api-key-32-characters-long'
$env:DATABASE_URL = 'sqlite://'
$env:COOKIE_SECURE = 'false'
Set-Location apps\core-api
..\..\.venv\Scripts\python.exe -m app.export_openapi
Set-Location ..\..
```

Generate TypeScript API types with Node.js:

```powershell
Set-Location packages\contracts
npm install
npm run generate:types
```

Do not place database credentials or Supabase API keys in the web app. The web
server must call the core API.
