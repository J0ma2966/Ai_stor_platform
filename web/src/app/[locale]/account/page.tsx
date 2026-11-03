"use client";
import { useTranslations } from "next-intl";
import { Link } from "@/i18n/navigation";
import { useAccountNav } from "@/components/CabNav";
import { useSession } from "@/lib/session";

export default function AccountHome() {
  const t = useTranslations("account");
  const { me, unread, cartQty } = useSession();
  const nav = useAccountNav().slice(1);
  const extra: Record<string, string> = {
    "/account/balance": t("tokensLeft", { n: me?.token_balance ?? 0 }),
    "/cart": cartQty ? String(cartQty) : "",
    "/account/notifications": unread ? String(unread) : "",
  };
  return (
    <>
      <h1>{t("title")}</h1>
      <p className="muted">{t("hello")} {me?.phone && <>· {t("phone")}: {me.phone}</>}</p>
      <ul className="tiles plain">
        {nav.map((n) => (
          <li key={n.href}>
            <Link href={n.href} className="card tile tile-link">
              <strong>{n.label}</strong>
              {extra[n.href] && <span className="num">{extra[n.href]}</span>}
            </Link>
          </li>
        ))}
      </ul>
    </>
  );
}
