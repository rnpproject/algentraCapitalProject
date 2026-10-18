"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { apiRequest } from "../lib/api";
import { AccountPerformanceDetails } from "./AccountPerformanceDetails";

type CopyAccount = { id:number; label:string; server:string; login:string; plan:string; execution_mode:string; owner_email:string };

export function AdminCopyAccountsPanel() {
  const router=useRouter();
  const [accounts,setAccounts]=useState<CopyAccount[]>([]);
  const [selected,setSelected]=useState<CopyAccount|null>(null);
  const [error,setError]=useState("");
  useEffect(()=>{let alive=true;apiRequest<{accounts:CopyAccount[]}>("admin/copy-accounts").then(data=>{if(alive)setAccounts(data.accounts??[])}).catch(reason=>{if(!alive)return;if(reason instanceof Error && reason.message.includes("admin"))router.replace("/admin/login");else setError(reason instanceof Error?reason.message:"Daftar akun tidak tersedia.")});return()=>{alive=false}},[router]);
  return <main className="page-shell dashboard-page" id="main-content"><div className="page-heading"><div><span className="eyebrow">ADMIN / COPY TRADING</span><h1>Akun Copy Trading</h1><p className="page-description">Pilih akun untuk membuka saldo, performa tujuh hari, dan riwayat transaksi.</p></div></div>
    {error&&<p className="status" role="alert">{error}</p>}
    <section className="panel"><div className="table-wrap"><table><thead><tr><th>ID</th><th>Nama akun</th><th>Pemilik</th><th>Login / Server</th><th></th></tr></thead><tbody>
      {accounts.map(account=><tr key={account.id}><td>{account.id}</td><td>{account.label}</td><td>{account.owner_email}</td><td>{account.login} · {account.server}</td><td><button type="button" className="secondary" onClick={()=>setSelected(account)}>Detail</button></td></tr>)}
      {!accounts.length&&!error&&<tr><td colSpan={5}>Memuat akun atau belum ada akun copy trading.</td></tr>}
    </tbody></table></div></section>
    {selected&&<div className="account-modal-backdrop" role="presentation" onMouseDown={event=>{if(event.target===event.currentTarget)setSelected(null)}}><section className="account-modal panel" role="dialog" aria-modal="true" aria-labelledby="copy-account-modal-title"><button className="secondary account-modal-close" type="button" onClick={()=>setSelected(null)} aria-label="Tutup detail">Tutup</button><span className="eyebrow">AKUN #{selected.id} · {selected.execution_mode}</span><h2 id="copy-account-modal-title">{selected.label}</h2><p>{selected.owner_email} · {selected.login} · {selected.server}</p><AccountPerformanceDetails accountId={selected.id} admin /></section></div>}
  </main>;
}
