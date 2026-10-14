"use client";

import { usePathname } from "next/navigation";

/**
 * Halaman 404 — port verbatim dari apps/core-api/app/templates/404.html
 * (backup stabil added2df). `requested_path` diganti `usePathname()`.
 */
export default function NotFound() {
  const pathname = usePathname() ?? "";

  return (
    <main id="main" tabIndex={-1} className="page-shell dashboard-page not-found-shell">
      <section className="not-found-card" aria-labelledby="not-found-title">
        <div className="not-found-copy">
          <span className="eyebrow">ALGENTRA CAPITAL / 404</span>
          <span className="not-found-code">HALAMAN TIDAK DITEMUKAN</span>
          <h1 id="not-found-title">Sinyalnya nyasar?</h1>
          <p>
            Chart-nya jalan, halamannya nggak ketemu. Tenang, ini cuma floating loss satu
            halaman—yuk balik ke area aman.
          </p>
          <code className="not-found-path">{pathname}</code>
          <div className="not-found-actions">
            <a href="/" aria-label="Kembali ke beranda">
              Kembali ke beranda
              <svg
                width="16"
                height="16"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
                aria-hidden="true"
              >
                <path d="M5 12h14M13 6l6 6-6 6"></path>
              </svg>
            </a>
            <a className="secondary" href="/login">
              Masuk
            </a>
          </div>
        </div>

        <svg
          className="not-found-art"
          viewBox="0 0 420 330"
          role="img"
          aria-labelledby="mascot-title mascot-description"
        >
          <title id="mascot-title">Trader kecil kebingungan melihat chart yang bergerak</title>
          <desc id="mascot-description">
            Chart candlestick XAUUSD bergerak sementara trader bertelinga bulat menatap layar dengan
            kopi dan ekspresi salah entry.
          </desc>
          <defs>
            <linearGradient id="chart-panel" x1="0" y1="0" x2="0" y2="1">
              <stop stopColor="#1c2b30" />
              <stop offset="1" stopColor="#111b20" />
            </linearGradient>
            <linearGradient id="trader-fur" x1="0" y1="0" x2="1" y2="1">
              <stop stopColor="#e7c99a" />
              <stop offset="1" stopColor="#bc9669" />
            </linearGradient>
            <linearGradient id="trader-hoodie" x1="0" y1="0" x2="1" y2="1">
              <stop stopColor="#93b7a0" />
              <stop offset="1" stopColor="#587966" />
            </linearGradient>
          </defs>
          <rect x="25" y="24" width="370" height="246" rx="19" fill="url(#chart-panel)" stroke="#3b5149" strokeWidth="2" />
          <path d="M26 62h368" stroke="#354640" />
          <rect x="43" y="36" width="18" height="14" rx="4" fill="#a5c6b522" stroke="#9db8a5" />
          <path d="M47 46v-4m4 4v-7m4 7v-5" stroke="#a5c6b5" strokeWidth="1.5" strokeLinecap="round" />
          <text x="69" y="48" fill="#e3ece5" fontSize="11" fontWeight="800" fontFamily="system-ui, sans-serif">
            XAUUSD
          </text>
          <text x="125" y="48" fill="#82948d" fontSize="9" fontFamily="system-ui, sans-serif">
            · M1
          </text>
          <circle cx="345" cy="43" r="3" fill="#8fc5a0" />
          <text x="353" y="47" fill="#9fc3a8" fontSize="8" fontWeight="700" fontFamily="system-ui, sans-serif">
            LIVE
          </text>
          <g className="chart-grid" stroke="#9ab3a012" strokeWidth="1">
            <path d="M49 88h326M49 119h326M49 150h326M49 181h326M49 212h326" />
            <path d="M80 78v144m48-144v144m48-144v144m48-144v144m48-144v144m48-144v144" />
          </g>
          <g fill="#71837b" fontSize="7" fontFamily="system-ui, sans-serif">
            <text x="31" y="91">2352</text>
            <text x="31" y="122">2350</text>
            <text x="31" y="153">2348</text>
            <text x="31" y="184">2346</text>
          </g>
          <path
            className="chart-trace"
            d="M54 184l20-15 18 9 20-37 19 15 19-25 18 29 20-12 17 18 20-39 19 10 18-19 20 32 19-16 18 21 18-38"
            fill="none"
            stroke="#a8cdb3"
            strokeWidth="2.5"
            strokeLinecap="round"
            strokeLinejoin="round"
            opacity=".72"
          />
          <g fill="none" strokeWidth="2" strokeLinecap="round">
            <g className="candle" stroke="#8fc5a0">
              <path d="M77 142v34" />
              <rect x="72" y="151" width="10" height="16" rx="1" fill="#8fc5a0" />
            </g>
            <g className="candle" stroke="#da8e88">
              <path d="M98 126v39" />
              <rect x="93" y="136" width="10" height="18" rx="1" fill="#da8e88" />
            </g>
            <g className="candle" stroke="#8fc5a0">
              <path d="M119 137v34" />
              <rect x="114" y="145" width="10" height="16" rx="1" fill="#8fc5a0" />
            </g>
            <g className="candle" stroke="#8fc5a0">
              <path d="M140 111v48" />
              <rect x="135" y="122" width="10" height="25" rx="1" fill="#8fc5a0" />
            </g>
            <g className="candle" stroke="#da8e88">
              <path d="M161 101v43" />
              <rect x="156" y="112" width="10" height="19" rx="1" fill="#da8e88" />
            </g>
            <g className="candle" stroke="#8fc5a0">
              <path d="M182 119v43" />
              <rect x="177" y="127" width="10" height="20" rx="1" fill="#8fc5a0" />
            </g>
            <g className="candle" stroke="#da8e88">
              <path d="M203 109v47" />
              <rect x="198" y="119" width="10" height="22" rx="1" fill="#da8e88" />
            </g>
            <g className="candle" stroke="#8fc5a0">
              <path d="M224 131v40" />
              <rect x="219" y="138" width="10" height="19" rx="1" fill="#8fc5a0" />
            </g>
            <g className="candle" stroke="#da8e88">
              <path d="M245 98v45" />
              <rect x="240" y="108" width="10" height="20" rx="1" fill="#da8e88" />
            </g>
            <g className="candle" stroke="#8fc5a0">
              <path d="M266 105v38" />
              <rect x="261" y="113" width="10" height="19" rx="1" fill="#da8e88" />
            </g>
            <g className="candle" stroke="#da8e88">
              <path d="M287 122v46" />
              <rect x="282" y="130" width="10" height="23" rx="1" fill="#da8e88" />
            </g>
          </g>
          <g className="chart-price">
            <path d="M300 171h74" stroke="#d9a27e" strokeWidth="1" strokeDasharray="3 3" />
            <rect x="339" y="163" width="40" height="16" rx="4" fill="#49352d" stroke="#986d52" />
            <text x="345" y="174" fill="#f0c5a5" fontSize="8" fontWeight="700" fontFamily="system-ui, sans-serif">
              2347.8
            </text>
          </g>
          <g className="speech-bubble">
            <path
              d="M262 70q0-7 8-7h101q8 0 8 8v25q0 8-8 8h-34l-11 11 2-11h-58q-8 0-8-8z"
              fill="#e1eadf"
              stroke="#91b49c"
              strokeWidth="1.5"
            />
            <text x="276" y="83" fill="#24352c" fontSize="8" fontWeight="800" fontFamily="system-ui, sans-serif">
              BUY THE DIP?
            </text>
            <text x="278" y="94" fill="#63776b" fontSize="7" fontFamily="system-ui, sans-serif">
              (katanya sih...)
            </text>
          </g>
          <ellipse className="trader-shadow" cx="286" cy="298" rx="67" ry="10" fill="#000" opacity=".3" />
          <g className="trader">
            <path d="M240 271q4-35 37-43h29q31 8 35 43v18h-101z" fill="url(#trader-hoodie)" stroke="#496757" strokeWidth="3" />
            <path d="M266 234q20 24 42 0" fill="none" stroke="#c4d7c5" strokeWidth="3" opacity=".65" />
            <path d="M279 255h14" stroke="#d2c69e" strokeWidth="3" strokeLinecap="round" />
            <circle cx="284" cy="176" r="39" fill="url(#trader-fur)" stroke="#98764f" strokeWidth="3" />
            <circle cx="254" cy="154" r="13" fill="#c4a174" stroke="#98764f" strokeWidth="3" />
            <circle cx="314" cy="154" r="13" fill="#c4a174" stroke="#98764f" strokeWidth="3" />
            <circle cx="254" cy="154" r="6" fill="#e8c9a0" />
            <circle cx="314" cy="154" r="6" fill="#e8c9a0" />
            <path d="M250 175q2-29 34-34 32 5 34 34" fill="none" stroke="#334b45" strokeWidth="6" strokeLinecap="round" />
            <rect x="246" y="168" width="9" height="22" rx="4" fill="#334b45" />
            <rect x="313" y="168" width="9" height="22" rx="4" fill="#334b45" />
            <path d="M264 176q7-5 13 0m14 0q7-5 13 0" fill="none" stroke="#634c38" strokeWidth="2.5" strokeLinecap="round" />
            <ellipse cx="272" cy="185" rx="3.5" ry="5" fill="#26352f" />
            <ellipse cx="300" cy="185" rx="3.5" ry="5" fill="#26352f" />
            <ellipse cx="263" cy="194" rx="6" ry="3" fill="#d99287" opacity=".7" />
            <ellipse cx="308" cy="194" rx="6" ry="3" fill="#d99287" opacity=".7" />
            <path d="M281 198q4-4 8 0" fill="none" stroke="#775742" strokeWidth="2.5" strokeLinecap="round" />
            <path className="sweat-drop" d="M327 164q-7 9 0 12 7-3 0-12z" fill="#93c6d0" />
            <path d="M239 257q-12 2-16-8" fill="none" stroke="#98764f" strokeWidth="9" strokeLinecap="round" />
            <path d="M329 251q13-1 18 9" fill="none" stroke="#98764f" strokeWidth="9" strokeLinecap="round" />
            <path d="M342 252h22v22q0 7-7 7h-8q-7 0-7-7z" fill="#eadbbd" stroke="#947f5d" strokeWidth="2" />
            <path d="M364 257q13-1 9 9-2 5-9 4" fill="none" stroke="#cdb990" strokeWidth="4" />
            <path
              className="coffee-steam"
              d="M349 247q-5-6 0-11t0-10m9 21q5-6 0-11t0-10"
              fill="none"
              stroke="#d5d6c6"
              strokeWidth="2"
              strokeLinecap="round"
            />
          </g>
        </svg>
      </section>
    </main>
  );
}
