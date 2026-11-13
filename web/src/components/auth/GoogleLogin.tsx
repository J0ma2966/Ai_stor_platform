"use client";
// Кнопка «Войти с Google» (Google Identity Services). Скрипт грузится только на
// странице входа; CSP пропускает его через 'strict-dynamic', iframe — через frame-src.
import { useEffect, useRef, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { loginGoogle } from "@/lib/api";
import { ErrorBox } from "@/components/ErrorBox";

type Gis = {
  accounts: {
    id: {
      initialize(o: { client_id: string; callback: (r: { credential: string }) => void; ux_mode?: string; context?: string }): void;
      renderButton(el: HTMLElement, o: Record<string, unknown>): void;
    };
  };
};

const SRC = "https://accounts.google.com/gsi/client";
let loading: Promise<Gis> | null = null;

function loadGis(): Promise<Gis> {
  const w = window as unknown as { google?: Gis };
  if (w.google?.accounts?.id) return Promise.resolve(w.google);
  if (!loading) {
    loading = new Promise((resolve, reject) => {
      const s = document.createElement("script");
      s.src = SRC;
      s.async = true;
      s.onload = () => (w.google ? resolve(w.google) : reject(new Error("gis")));
      s.onerror = () => {
        loading = null;
        reject(new Error("gis"));
      };
      document.head.appendChild(s);
    });
  }
  return loading;
}

export function GoogleLogin({ clientId, onSignedIn }: { clientId: string; onSignedIn: () => Promise<void> }) {
  const locale = useLocale();
  const t = useTranslations("login");
  const box = useRef<HTMLDivElement>(null);
  const [error, setError] = useState<unknown>(null);
  const [failed, setFailed] = useState(false);
  const cb = useRef(onSignedIn);
  cb.current = onSignedIn;

  useEffect(() => {
    let alive = true;
    loadGis()
      .then((g) => {
        if (!alive || !box.current) return;
        g.accounts.id.initialize({
          client_id: clientId,
          ux_mode: "popup",
          context: "signin",
          callback: async ({ credential }) => {
            setError(null);
            try {
              await loginGoogle(credential);
              await cb.current();
            } catch (err) {
              setError(err);
            }
          },
        });
        const width = Math.min(box.current.clientWidth || 360, 400);
        g.accounts.id.renderButton(box.current, {
          type: "standard",
          theme: "outline",
          size: "large",
          shape: "pill",
          text: "continue_with",
          logo_alignment: "center",
          width,
          locale: locale === "uz" ? "uz" : "ru",
        });
      })
      .catch(() => alive && setFailed(true));
    return () => {
      alive = false;
    };
  }, [clientId, locale]);

  return (
    <div className="stack-sm">
      <div ref={box} className="auth-google" data-testid="google-button" />
      {failed && <p className="muted small">{t("googleUnavailable")}</p>}
      <ErrorBox error={error} />
    </div>
  );
}
