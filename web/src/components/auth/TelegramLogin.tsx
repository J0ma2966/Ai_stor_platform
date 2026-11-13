"use client";
// Вход через нашего Telegram-бота: сайт получает ссылку t.me/<бот>?start=login_…,
// человек подтверждает вход в боте (новый — делится номером), сайт опрашивает API.
import { useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { ApiError, pollTelegramLogin, startTelegramLogin, type TelegramStart } from "@/lib/api";
import { ErrorBox } from "@/components/ErrorBox";

const POLL_MS = 2000;

export function TelegramIcon() {
  return (
    <svg width="22" height="22" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <circle cx="12" cy="12" r="12" fill="#fff" />
      <path
        fill="#229ED9"
        d="M5.3 11.7c3.5-1.5 5.8-2.5 7-3 3.3-1.4 4-1.6 4.5-1.6.1 0 .3 0 .5.2.1.1.2.3.2.4v.6c-.2 2-1 6.9-1.5 9.2-.2 1-.6 1.3-.9 1.3-.8.1-1.4-.5-2.1-1l-3-2c-1.3-.9-.5-1.4.3-2.2.2-.2 3.6-3.3 3.7-3.6v-.2c-.1-.1-.2 0-.3 0-.1 0-2 1.3-5.6 3.7-.5.4-1 .5-1.5.5-.5 0-1.4-.3-2.1-.5-.8-.3-1.5-.4-1.4-.9 0-.2.4-.5 1-.9z"
      />
    </svg>
  );
}

export function TelegramLogin({ onSignedIn }: { onSignedIn: () => Promise<void> }) {
  const t = useTranslations("tgAuth");
  const [attempt, setAttempt] = useState<TelegramStart | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const done = useRef(false);

  // Опрос, пока человек подтверждает вход в Telegram.
  useEffect(() => {
    if (!attempt) return;
    let alive = true;
    let timer: ReturnType<typeof setTimeout>;
    const tick = async () => {
      try {
        const s = await pollTelegramLogin(attempt);
        if (!alive) return;
        if (s) {
          done.current = true;
          await onSignedIn();
          return;
        }
      } catch (err) {
        if (!alive) return;
        if (err instanceof ApiError && (err.code === "telegram_login_expired" || err.code === "account_blocked")) {
          setAttempt(null);
          setError(err);
          return;
        }
        // сеть или лимит — пробуем дальше, ссылка ещё жива
      }
      timer = setTimeout(tick, POLL_MS);
    };
    timer = setTimeout(tick, POLL_MS);
    return () => {
      alive = false;
      clearTimeout(timer);
    };
  }, [attempt, onSignedIn]);

  async function start() {
    setError(null);
    setBusy(true);
    // Вкладку открываем сразу по клику: после await браузер счёл бы её всплывающим окном.
    const tab = typeof window !== "undefined" ? window.open("", "_blank") : null;
    try {
      const a = await startTelegramLogin();
      if (tab) {
        tab.opener = null;
        tab.location.href = a.url;
      }
      setAttempt(a);
    } catch (err) {
      tab?.close();
      setError(err);
    } finally {
      setBusy(false);
    }
  }

  if (attempt) {
    return (
      <div className="auth-wait" role="status" aria-live="polite">
        <span className="auth-spinner" aria-hidden="true" />
        <div className="stack-sm">
          <strong>{t("waitTitle")}</strong>
          <p className="muted small">{t("waitText")}</p>
          <div className="auth-links">
            <a className="btn-link" href={attempt.url} target="_blank" rel="noopener noreferrer">{t("openAgain")}</a>
            <button type="button" className="btn-link" onClick={() => setAttempt(null)}>{t("cancel")}</button>
          </div>
        </div>
      </div>
    );
  }
  return (
    <div className="stack-sm">
      <button type="button" className="btn auth-btn auth-btn-telegram" onClick={start} disabled={busy}>
        <TelegramIcon />
        {t("button")}
      </button>
      <ErrorBox error={error} />
    </div>
  );
}
