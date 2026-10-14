import type { Metadata } from "next";
import PublicPage from "./components/PublicPage";

export const metadata: Metadata = {
  title: "Algentra Capital | Copy Trading Otomatis Berbasis AI",
};

export default function HomePage() {
  return <PublicPage />;
}
