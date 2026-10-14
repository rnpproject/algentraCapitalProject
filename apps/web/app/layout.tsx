import type { Metadata } from "next";
import { Poppins, Plus_Jakarta_Sans } from "next/font/google";
import Script from "next/script";
import "./globals.css";

const poppins = Poppins({
  subsets: ["latin"],
  weight: ["400", "600", "700", "800"],
  variable: "--font-heading",
  display: "swap",
});

const plusJakartaSans = Plus_Jakarta_Sans({
  subsets: ["latin"],
  weight: ["400", "500", "600", "700", "800"],
  variable: "--font-body",
  display: "swap",
});

export const metadata: Metadata = {
  title: "Algentra Capital | AI Copy Trading",
  description:
    "Algentra Capital membantu membaca sinyal XAUUSD dengan AI, menjalankan instruksi melalui EA MetaTrader, dan menyalin posisi ke akun Copy Trading.",
  themeColor: "#0d1a24",
};

// Mirror base.html: theme read from cookie/localStorage, fallback dark, applied
// before paint to avoid a flash of the wrong theme.
const themeInitScript = `(function () {
  try {
    var cookieTheme = (document.cookie.match(/(?:^|;)\\s*theme=(dark|light)/) || [])[1];
    var stored = null;
    try { stored = localStorage.getItem("theme"); } catch (e) { /* cookie keeps the preference */ }
    var selected = stored === "dark" || stored === "light" ? stored : (cookieTheme === "light" ? "light" : "dark");
    document.documentElement.setAttribute("data-theme", selected);
    var meta = document.querySelector('meta[name="theme-color"]');
    if (meta) meta.setAttribute("content", selected === "light" ? "#f2f2f2" : "#0d1a24");
  } catch (e) { /* keep server-rendered default */ }
})();`;

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html
      lang="id"
      data-theme="dark"
      suppressHydrationWarning
      className={`${poppins.variable} ${plusJakartaSans.variable}`}
    >
      <body className={plusJakartaSans.className}>
        <Script
          id="theme-init"
          strategy="beforeInteractive"
          dangerouslySetInnerHTML={{ __html: themeInitScript }}
        />
        {children}
      </body>
    </html>
  );
}
