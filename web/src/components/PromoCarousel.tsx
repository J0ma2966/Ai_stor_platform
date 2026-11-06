"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Link } from "@/i18n/navigation";
import { cardImage, type Product } from "@/lib/catalog";
import { formatSum } from "@/lib/money";
import { formatRating } from "@/lib/reviews";
import { Icon } from "./Icon";

// Сколько показывается каждый товар, прежде чем карусель перейдёт к следующему.
export const ROTATE_MS = 5000;

// Карусель «Реклама» на главной: товары, продвижение которых одобрил суперадмин
// (GET /v1/promotions). Каждый слайд помечен «Реклама».
//
// Автопрокрутка по правилам доступности (WAI, ui-ux-pro-max «Carousel / Auto-Rotation»):
// один таймер; пауза при наведении, фокусе внутри, уходе карусели за экран и скрытой
// вкладке; кнопки «назад/вперёд» и «пауза/пуск»; prefers-reduced-motion — без
// автопрокрутки и без анимации; слайды за пределами видимой области — inert.
export function PromoCarousel({ items }: { items: Product[] }) {
  const t = useTranslations("promo");
  const locale = useLocale();
  const root = useRef<HTMLElement>(null);
  const [index, setIndex] = useState(0);
  const [perView, setPerView] = useState(1);
  const [paused, setPaused] = useState(false); // пауза кнопкой — до следующего нажатия
  const [held, setHeld] = useState(false); // наведение / фокус / за экраном / скрытая вкладка
  const [reduced, setReduced] = useState(false);
  const n = items.length;
  const last = Math.max(0, n - perView);
  const rotating = last > 0;

  useEffect(() => {
    const wide = matchMedia("(min-width: 900px)");
    const rm = matchMedia("(prefers-reduced-motion: reduce)");
    const sync = () => {
      setPerView(wide.matches ? 2 : 1);
      setReduced(rm.matches);
    };
    sync();
    wide.addEventListener("change", sync);
    rm.addEventListener("change", sync);
    return () => {
      wide.removeEventListener("change", sync);
      rm.removeEventListener("change", sync);
    };
  }, []);

  useEffect(() => setIndex((i) => Math.min(i, last)), [last]);

  // наведение, фокус внутри, за экраном, скрытая вкладка — карусель стоит
  useEffect(() => {
    const el = root.current;
    if (!el) return;
    const holdBy = new Set<string>(); // причины остановки; крутится, только когда их нет
    const apply = () => setHeld(holdBy.size > 0);
    const io = typeof IntersectionObserver === "undefined" ? null : new IntersectionObserver(([e]) => {
      if (e.isIntersecting) holdBy.delete("offscreen"); else holdBy.add("offscreen");
      apply();
    });
    io?.observe(el);
    const onVis = () => { if (document.hidden) holdBy.add("hidden"); else holdBy.delete("hidden"); apply(); };
    const enter = () => { holdBy.add("hover"); apply(); };
    const leave = () => { holdBy.delete("hover"); apply(); };
    const focusIn = () => { holdBy.add("focus"); apply(); };
    const focusOut = (e: FocusEvent) => { if (!el.contains(e.relatedTarget as Node | null)) { holdBy.delete("focus"); apply(); } };
    document.addEventListener("visibilitychange", onVis);
    el.addEventListener("pointerenter", enter);
    el.addEventListener("pointerleave", leave);
    el.addEventListener("focusin", focusIn);
    el.addEventListener("focusout", focusOut);
    return () => {
      io?.disconnect();
      document.removeEventListener("visibilitychange", onVis);
      el.removeEventListener("pointerenter", enter);
      el.removeEventListener("pointerleave", leave);
      el.removeEventListener("focusin", focusIn);
      el.removeEventListener("focusout", focusOut);
    };
  }, []);

  const go = useCallback((to: number) => setIndex(to < 0 ? last : to > last ? 0 : to), [last]);

  // один таймер: перезапускается при каждом переходе, чтобы ручной переход давал полный интервал
  const auto = rotating && !paused && !held && !reduced;
  useEffect(() => {
    if (!auto) return;
    const id = window.setTimeout(() => go(index + 1), ROTATE_MS);
    return () => window.clearTimeout(id);
  }, [auto, index, go]);

  // свайп на телефоне
  const startX = useRef<number | null>(null);
  const onPointerDown = (e: React.PointerEvent) => { if (e.pointerType !== "mouse") startX.current = e.clientX; };
  const onPointerUp = (e: React.PointerEvent) => {
    if (startX.current === null) return;
    const dx = e.clientX - startX.current;
    startX.current = null;
    if (Math.abs(dx) > 40) go(index + (dx < 0 ? 1 : -1));
  };

  if (n === 0) return null;
  return (
    <section ref={root} className="ad-carousel" aria-roledescription={t("carousel")} aria-labelledby="h-promo">
      <div className="section-head">
        <h2 id="h-promo" className="row ad-head">
          <span className="ad-chip"><Icon name="megaphone" size={16} strokeWidth={2} />{t("label")}</span>
          {t("title")}
        </h2>
        {rotating && (
          <div className="row ad-controls">
            <button type="button" className="ad-ctl" onClick={() => go(index - 1)} aria-label={t("prev")}><Icon name="chevronLeft" size={20} strokeWidth={2.2} /></button>
            <button type="button" className="ad-ctl" onClick={() => setPaused((p) => !p)} aria-label={paused || reduced ? t("play") : t("pause")} aria-pressed={paused}
              disabled={reduced} title={reduced ? t("reducedMotion") : undefined}>
              <Icon name={paused || reduced ? "play" : "pause"} size={18} strokeWidth={2.2} />
            </button>
            <button type="button" className="ad-ctl" onClick={() => go(index + 1)} aria-label={t("next")}><Icon name="chevronRight" size={20} strokeWidth={2.2} /></button>
          </div>
        )}
      </div>
      <div className="ad-viewport" onPointerDown={onPointerDown} onPointerUp={onPointerUp} onPointerCancel={() => (startX.current = null)}>
        <ul className="ad-track plain" aria-live={auto ? "off" : "polite"}
          style={{ transform: `translateX(-${(index * 100) / perView}%)`, ["--per" as string]: perView }}>
          {items.map((p, i) => {
            const visible = i >= index && i < index + perView;
            const img = cardImage(p);
            return (
              <li key={p.id} className="ad-slide" role="group" aria-roledescription={t("slide")} aria-label={t("position", { n: i + 1, total: n })}
                aria-hidden={!visible} inert={!visible}>
                <Link href={`/product/${p.id}`} className="ad-card">
                  <div className="ad-img">
                    {img ? <img src={img} alt={p.title} loading={i < perView ? "eager" : "lazy"} width={300} height={400} /> : <div className="img-ph" aria-hidden="true" />}
                  </div>
                  <div className="ad-body">
                    <span className="ad-chip ad-chip-sm">{t("label")}</span>
                    <span className="ad-store">{p.store_name}</span>
                    <span className="ad-name">{p.title}</span>
                    {p.brand && p.brand !== p.store_name && <span className="muted small">{p.brand}</span>}
                    {!!p.rating_count && (
                      <span className="small"><span aria-hidden="true" className="ad-star">★</span> {formatRating(p.rating_avg ?? 0)} <span className="muted">({p.rating_count})</span></span>
                    )}
                    <span className="ad-price">{formatSum(p.price, locale)}</span>
                    <span className="btn btn-primary ad-go">{t("open")} <Icon name="arrow" size={18} strokeWidth={2.2} /></span>
                  </div>
                </Link>
              </li>
            );
          })}
        </ul>
      </div>
      {rotating && (
        <div className="ad-dots" role="group" aria-label={t("choose")}>
          {Array.from({ length: last + 1 }, (_, i) => (
            <button key={i} type="button" className={i === index ? "ad-dot ad-dot-on" : "ad-dot"} onClick={() => go(i)}
              aria-label={t("goTo", { n: i + 1 })} aria-current={i === index ? "true" : undefined} />
          ))}
        </div>
      )}
    </section>
  );
}
