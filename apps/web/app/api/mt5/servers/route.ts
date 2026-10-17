import { NextRequest, NextResponse } from "next/server";

export const runtime = "nodejs";

type DirectoryCompany = {
  companyName?: unknown;
  results?: Array<{ name?: unknown }>;
};

export async function GET(request: NextRequest) {
  const query = (request.nextUrl.searchParams.get("q") ?? "").trim();
  if (query.length < 3 || query.length > 96) {
    return NextResponse.json({ servers: [] });
  }

  const url = new URL(
    process.env.MT5_BROKER_DIRECTORY_URL || "https://mt5.mtapi.io/Search"
  );
  if (url.protocol !== "https:" || url.username || url.password || url.hash) {
    return NextResponse.json(
      { error: "Direktori broker tidak dikonfigurasi dengan aman." },
      { status: 500 }
    );
  }
  url.searchParams.set("company", query);

  try {
    const response = await fetch(url, {
      headers: { Accept: "application/json" },
      next: { revalidate: 1800 },
      signal: AbortSignal.timeout(8000),
    });
    if (!response.ok) {
      return NextResponse.json(
        { error: "Pencarian direktori broker sedang tidak tersedia." },
        { status: 502 }
      );
    }

    const payload: unknown = await response.json();
    const companies = Array.isArray(payload)
      ? (payload as DirectoryCompany[])
      : [];
    const seen = new Set<string>();
    const needle = query.toLocaleLowerCase();
    const servers = companies
      .flatMap((company) =>
        (Array.isArray(company.results) ? company.results : []).flatMap(
          (server) => {
            const name = typeof server.name === "string" ? server.name.trim() : "";
            const companyName =
              typeof company.companyName === "string"
                ? company.companyName.trim()
                : "";
            const key = `${companyName.toLowerCase()}|${name.toLowerCase()}`;
            if (!name || !companyName || seen.has(key)) return [];
            seen.add(key);
            return [{ name, company: companyName }];
          }
        )
      )
      .sort((a, b) => {
        const aExact = a.name.toLocaleLowerCase() === needle ? 0 : 1;
        const bExact = b.name.toLocaleLowerCase() === needle ? 0 : 1;
        return aExact - bExact || a.name.localeCompare(b.name);
      })
      .slice(0, 200);

    return NextResponse.json({ servers });
  } catch {
    return NextResponse.json(
      { error: "Pencarian direktori broker gagal. Coba lagi sebentar." },
      { status: 502 }
    );
  }
}
