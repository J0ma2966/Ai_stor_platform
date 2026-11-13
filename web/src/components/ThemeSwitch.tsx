"use client";
import { useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { applyTheme, THEMES, type Theme } from "@/lib/theme";
import { Icon, type IconName } from "./Icon";

const icon: Record<Theme, IconName> = { system: "contrast", light: "sun", dark: "moon" };
const label = { system: "app.themeSystem", light: "app.themeLight", dark: "app.themeDark" } as const;

/** Кнопка в шапке: меню из трёх вариантов (как в системе / светлая / тёмная). */
export function ThemeSwitch({ initial }: { initial: Theme }) {
  const t = useTranslations();
  const [theme, setTheme] = useState<Theme>(initial);
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const close = (e: Event) => {
      if (e instanceof KeyboardEvent ? e.key === "Escape" : !box.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("pointerdown", close);
    document.addEventListener("keydown", close);
    return () => {
      document.removeEventListener("pointerdown", close);
      document.removeEventListener("keydown", close);
    };
  }, [open]);

  function pick(v: Theme) {
    setTheme(v);
    applyTheme(v);
    setOpen(false);
  }

  return (
    <div className="hdr-user theme-switch" ref={box}>
      <button
        type="button"
        className="hdr-btn"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={`${t("app.theme")}: ${t(label[theme])}`}
        title={`${t("app.theme")}: ${t(label[theme])}`}
        onClick={() => setOpen((v) => !v)}
      >
        <Icon name={icon[theme]} />
        <span className="hdr-label">{t("app.theme")}</span>
      </button>
      {open && (
        <ul className="menu" role="menu" aria-label={t("app.theme")}>
          {THEMES.map((v) => (
            <li role="none" key={v}>
              <button role="menuitemradio" aria-checked={theme === v} type="button" className="btn-link theme-opt" onClick={() => pick(v)}>
                <Icon name={icon[v]} size={18} />
                <span>{t(label[v])}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
