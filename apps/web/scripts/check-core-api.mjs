// Diagnostik koneksi Core API dari mesin Anda (butuh Node 18+).
// Pakai:  CORE_API_ORIGIN=https://... CF_ACCESS_CLIENT_ID=... CF_ACCESS_CLIENT_SECRET=... node scripts/check-core-api.mjs
const origin = (process.env.CORE_API_ORIGIN ?? "").replace(/\/+$/, "");
const id = process.env.CF_ACCESS_CLIENT_ID;
const secret = process.env.CF_ACCESS_CLIENT_SECRET;
if (!origin || !id || !secret) {
  console.error("Isi CORE_API_ORIGIN, CF_ACCESS_CLIENT_ID, CF_ACCESS_CLIENT_SECRET (pakai tanda '=', underscore, bukan 'CF-Access-Client-Id:').");
  process.exit(1);
}
const headers = { "cf-access-client-id": id, "cf-access-client-secret": secret, accept: "application/json" };
const times = [];
for (let i = 1; i <= 8; i += 1) {
  const t0 = performance.now();
  try {
    const res = await fetch(`${origin}/health`, { headers, signal: AbortSignal.timeout(10000) });
    const ms = Math.round(performance.now() - t0);
    times.push(ms);
    console.log(`#${i} /health -> HTTP ${res.status} dalam ${ms} ms${res.headers.get("cf-ray") ? ` (cf-ray ${res.headers.get("cf-ray")})` : ""}`);
  } catch (error) {
    console.log(`#${i} /health -> GAGAL (${error.name}: ${error.message})`);
  }
  await new Promise((r) => setTimeout(r, 500));
}
if (times.length) {
  const sorted = [...times].sort((a, b) => a - b);
  console.log(`\nmin ${sorted[0]} ms · median ${sorted[Math.floor(sorted.length / 2)]} ms · max ${sorted.at(-1)} ms · berhasil ${times.length}/8`);
  console.log("Catatan: median >800 ms atau ada yang gagal berarti jalur Cloudflare Tunnel/Access-lah yang lambat, bukan frontend.");
}
