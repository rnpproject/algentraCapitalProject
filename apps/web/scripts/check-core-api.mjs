// Diagnostik koneksi Core API dari mesin Anda (butuh Node 18+).
// Pakai:  API_ORIGIN=https://... node scripts/check-core-api.mjs
const origin = (process.env.API_ORIGIN ?? "").replace(/\/+$/, "");
if (!origin) {
  console.error("Isi API_ORIGIN.");
  process.exit(1);
}
const headers = { accept: "application/json" };
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
  console.log("Catatan: median >800 ms atau ada yang gagal berarti origin API atau jalur jaringan perlu diperiksa.");
}
