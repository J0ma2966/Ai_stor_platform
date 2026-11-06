"use client";
import { useState } from "react";
import { useTranslations } from "next-intl";
import type { Angle } from "@/lib/tryon";

// Примеры фото для AI-примерки: как надо (3 ракурса) и типичные ошибки.
// Рисунки — схематичные SVG, а не фото людей: без прав на чужие снимки и
// без биометрии в бандле. Геометрия (viewBox 100×150) повторена в мобильном
// приложении (mobile/app-client/.../PhotoGuide.kt) — меняйте вместе.

export type Flaw = "cropped" | "dark" | "baggy" | "crowd" | "selfie" | "covered";
export const FLAWS: Flaw[] = ["cropped", "dark", "baggy", "crowd", "selfie", "covered"];

type Pose = Angle;

// Фигура человека в полный рост; cx — центр по горизонтали.
function Person({ pose, cx = 50, silhouette = false, hideArms = false }: { pose: Pose; cx?: number; silhouette?: boolean; hideArms?: boolean }) {
  const d = cx - 50;
  const c = (cls: string) => (silhouette ? "pg-sil" : cls);
  if (pose === "side") {
    return (
      <g transform={`translate(${d} 0)`}>
        <path className={c("pg-bottom")} d="M44 76 L56 76 L55 133 L46 133 Z" />
        <ellipse className={c("pg-shoe")} cx="52" cy="134" rx="7" ry="2.6" />
        <path className={c("pg-top")} d="M44 40 Q51 36 57 41 L56 77 L44 77 Z" />
        <rect className={c("pg-skin")} x="47.5" y="32" width="5" height="7" rx="2" />
        <ellipse className={c("pg-skin")} cx="50" cy="24" rx="8.5" ry="10.5" />
        <circle className={c("pg-skin")} cx="58" cy="25" r="1.8" />
        <path className={c("pg-hair")} d="M41.5 25 Q41 13 50 13 Q58 13 58.5 20 L50 19 Q46 22 46 29 Z" />
        {!hideArms && <path className={c("pg-arm")} d="M50 43 L51 80" />}
      </g>
    );
  }
  const back = pose === "back";
  return (
    <g transform={`translate(${d} 0)`}>
      <path className={c("pg-bottom")} d="M39 76 L61 76 L60 133 L52 133 L50 90 L48 133 L40 133 Z" />
      <ellipse className={c("pg-shoe")} cx="44" cy="134" rx="5.5" ry="2.4" />
      <ellipse className={c("pg-shoe")} cx="56" cy="134" rx="5.5" ry="2.4" />
      {!hideArms && (
        <>
          <path className={c("pg-arm")} d="M38 43 L29 80" />
          <path className={c("pg-arm")} d="M62 43 L71 80" />
        </>
      )}
      <path className={c("pg-top")} d="M36 41 Q50 35 64 41 L61 77 L39 77 Z" />
      <rect className={c("pg-skin")} x="47" y="32" width="6" height="7" rx="2" />
      <ellipse className={c(back ? "pg-hair" : "pg-skin")} cx="50" cy="24" rx="9" ry="10.5" />
      {!back && <path className={c("pg-hair")} d="M41 23 Q41 13 50 13 Q59 13 59 23 Q55 17 50 17 Q45 17 41 23 Z" />}
    </g>
  );
}

function Mark({ ok }: { ok: boolean }) {
  return (
    <g className={ok ? "pg-mark-ok" : "pg-mark-bad"}>
      <circle cx="86" cy="14" r="9" />
      {ok ? <path className="pg-mark-ico" d="M81.5 14.5 L85 18 L91 10.5" /> : <path className="pg-mark-ico" d="M82 10 L90 18 M90 10 L82 18" />}
    </g>
  );
}

// Одна карточка-пример. ghost — бледный контур для пустого слота загрузки.
export function PoseFigure({ pose = "front", flaw, mark, ghost = false, id }: { pose?: Pose; flaw?: Flaw; mark?: boolean; ghost?: boolean; id: string }) {
  const clip = `pg-clip-${id}`;
  const dark = flaw === "dark";
  return (
    <svg viewBox="0 0 100 150" className={`pose-figure${ghost ? " pose-ghost" : ""}`} aria-hidden="true" focusable="false">
      <defs>
        <clipPath id={clip}>
          <rect width="100" height="150" rx="10" />
        </clipPath>
      </defs>
      <g clipPath={`url(#${clip})`}>
        <rect className={dark ? "pg-bg-dark" : "pg-bg"} width="100" height="150" />
        <rect className={dark ? "pg-floor-dark" : "pg-floor"} y="128" width="100" height="22" />
        {flaw === "crowd" && (
          <g className="pg-clutter">
            <rect x="4" y="20" width="26" height="4" rx="1" />
            <rect x="6" y="10" width="6" height="10" rx="1" />
            <rect x="14" y="13" width="5" height="7" rx="1" />
            <rect x="70" y="40" width="24" height="30" rx="2" />
            <circle cx="12" cy="104" r="9" />
            <rect x="10" y="108" width="4" height="20" />
          </g>
        )}
        {dark && <rect className="pg-window" x="26" y="16" width="48" height="96" rx="3" />}
        {flaw === "cropped" ? (
          <g transform="translate(50 52) scale(1.55) translate(-50 -40)">
            <Person pose={pose} />
          </g>
        ) : flaw === "crowd" ? (
          <>
            <g transform="translate(78 30) scale(.8) translate(-50 0)">
              <Person pose="front" />
            </g>
            <Person pose={pose} cx={40} />
          </>
        ) : flaw === "baggy" ? (
          <>
            <Person pose={pose} hideArms />
            <path className="pg-coat" d="M33 39 Q50 33 67 39 L79 116 L21 116 Z" />
            <path className="pg-coat-line" d="M50 38 L50 116" />
          </>
        ) : flaw === "selfie" ? (
          <>
            <Person pose={pose} hideArms />
            <path className="pg-arm" d="M38 43 L29 80" />
            <path className="pg-arm" d="M62 43 L68 31 L58 26" />
            <rect className="pg-phone" x="47" y="14" width="13" height="19" rx="2.5" />
            <rect className="pg-mirror" x="4" y="4" width="92" height="142" rx="8" />
          </>
        ) : flaw === "covered" ? (
          <>
            <Person pose={pose} hideArms />
            <path className="pg-strap" d="M40 42 Q50 52 60 42" />
            <rect className="pg-bag" x="31" y="56" width="38" height="30" rx="5" />
            <path className="pg-arm" d="M38 43 L33 62 L60 64" />
            <path className="pg-arm" d="M62 43 L67 62 L40 66" />
          </>
        ) : (
          <Person pose={pose} silhouette={dark} />
        )}
      </g>
      {mark !== undefined && <Mark ok={mark} />}
    </svg>
  );
}

// Блок «Примеры фото» в мастере примерки и в «Моих фото».
export function PhotoGuide({ defaultOpen = true }: { defaultOpen?: boolean }) {
  const t = useTranslations("tryon.photoGuide");
  const ta = useTranslations("tryon.angle");
  const [open, setOpen] = useState(defaultOpen);
  return (
    <details className="photo-guide" open={open} onToggle={(e) => setOpen((e.currentTarget as HTMLDetailsElement).open)}>
      <summary>{t("title")}</summary>
      <div className="stack">
        <section aria-labelledby="pg-good">
          <h3 id="pg-good" className="pg-head pg-head-ok">{t("good")}</h3>
          <p className="small muted">{t("goodHint")}</p>
          <ul className="pg-grid pg-grid-good">
            {(["front", "side", "back"] as const).map((a) => (
              <li key={a}>
                <PoseFigure pose={a} mark id={`good-${a}`} />
                <span className="small">{ta(a)}</span>
              </li>
            ))}
          </ul>
        </section>
        <section aria-labelledby="pg-bad">
          <h3 id="pg-bad" className="pg-head pg-head-bad">{t("bad")}</h3>
          <ul className="pg-grid">
            {FLAWS.map((f) => (
              <li key={f}>
                <PoseFigure flaw={f} mark={false} id={`bad-${f}`} />
                <span className="small">{t(`flaw.${f}`)}</span>
              </li>
            ))}
          </ul>
        </section>
        <p className="small pg-tip">{t("tip")}</p>
      </div>
    </details>
  );
}
