import { AuthForm } from "../../components/AuthForm";
import { SiteFrame } from "../../components/SiteFrame";

export const metadata = { robots: "noindex, nofollow" };

export default function AdminLoginPage() { return <SiteFrame><AuthForm mode="admin" /></SiteFrame>; }
