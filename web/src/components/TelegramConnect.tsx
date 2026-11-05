"use client";
import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { getTelegram, telegramLink, telegramUnlink, type TelegramStatus } from "@/lib/storeCab";
import { ErrorBox } from "./ErrorBox";

// Подключение Telegram-бота: новые заказы приходят в чат с кнопкой «Подтвердить».
export function TelegramConnect() {
  const t = useTranslations("storeProfile");
  const [st, setSt] = useState<TelegramStatus | null>(null);
  const [link, setLink] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const load = useCallback(() => {
    getTelegram().then((s) => {
      setSt(s);
      if (s.connected) setLink("");
    }, setError);
  }, []);
  useEffect(load, [load]);
  // пока открыта ссылка подключения — проверяем, нажал ли магазин «Старт»
  useEffect(() => {
    if (!link) return;
    const timer = setInterval(() => document.visibilityState === "visible" && load(), 5000);
    return () => clearInterval(timer);
  }, [link, load]);

  async function run(fn: () => Promise<unknown>) {
    setBusy(true);
    setError(null);
    try {
      await fn();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }

  if (!st) return null;
  return (
    <section className="card stack-sm tg-box" aria-labelledby="tg-title">
      <h2 id="tg-title">{t("tgTitle")}</h2>
      <p className="muted small">{t("tgText")}</p>
      {!st.available && <p className="small">{t("tgUnavailable")}</p>}
      {st.available && st.connected && (
        <div className="row wrap">
          <span className="badge badge-ok">✓ {t("tgConnected")}</span>
          <button type="button" className="btn btn-sm btn-secondary" disabled={busy}
            onClick={() => run(async () => { await telegramUnlink(); load(); })}>{t("tgDisconnect")}</button>
        </div>
      )}
      {st.available && !st.connected && !link && (
        <button type="button" className="btn btn-primary" disabled={busy}
          onClick={() => run(async () => setLink((await telegramLink()).url))}>{t("tgConnect")}</button>
      )}
      {st.available && !st.connected && link && (
        <div className="stack-xs">
          <a className="btn btn-primary" href={link} target="_blank" rel="noopener noreferrer">{t("tgOpen")} ↗</a>
          <p className="muted small">{t("tgLinkHint")}</p>
          <button type="button" className="btn-link small" onClick={load}>{t("tgRefresh")}</button>
        </div>
      )}
      <ErrorBox error={error} />
    </section>
  );
}
