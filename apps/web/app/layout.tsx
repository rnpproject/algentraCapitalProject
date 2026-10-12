import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Algentra Capital | Copy trading terkelola",
  description: "Infrastruktur copy trading dan pemantauan akun MT5 Algentra Capital."
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="id"><body>{children}</body></html>;
}
