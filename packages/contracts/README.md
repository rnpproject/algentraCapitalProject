# API contracts

`openapi.json` is the checked-in contract for the versioned API. The existing
`/api/...` endpoints remain available for installed clients while consumers move
to `/v1/...` paths. Dashboard operations are under `/v1/dashboard`; public
performance endpoints are under `/v1/public`.

`openapi.json` is the source of truth for generated client types. Update it when
the Go API contract changes, then regenerate the TypeScript types:

```powershell
Set-Location packages\contracts
npm install
npm run generate:types
```

Do not place database credentials or Supabase API keys in the web app. The web
server must call the core API.
