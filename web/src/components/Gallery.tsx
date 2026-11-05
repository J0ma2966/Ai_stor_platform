"use client";
import { useState } from "react";
import { useTranslations } from "next-intl";

const ORDER = ["front", "side", "back"] as const;

// Фото товара: спереди, сбоку, сзади (ТЗ 4.4). Миниатюры — кнопки, работают с клавиатуры.
type Images = Partial<Record<(typeof ORDER)[number], string>>;

// thumbs — уменьшенные копии для кнопок-миниатюр (без них берутся большие фото).
export function Gallery({ images, thumbs = {}, title }: { images: Images; thumbs?: Images; title: string }) {
  const t = useTranslations("product");
  const list = ORDER.filter((a) => images[a]).map((a) => ({ angle: a, url: images[a] as string, thumb: thumbs[a] ?? (images[a] as string) }));
  const [i, setI] = useState(0);
  if (!list.length) return <div className="gallery"><div className="img-ph gallery-main" aria-hidden="true" /></div>;
  const cur = list[Math.min(i, list.length - 1)];
  return (
    <div className="gallery">
      <img className="gallery-main" src={cur.url} alt={`${title} — ${t(cur.angle)}`} width={600} height={800} />
      {list.length > 1 && (
        <ul className="gallery-thumbs plain" aria-label={t("photo", { n: i + 1, total: list.length })}>
          {list.map((x, j) => (
            <li key={x.angle}>
              <button type="button" className={j === i ? "thumb thumb-on" : "thumb"} aria-pressed={j === i} aria-label={t(x.angle)} onClick={() => setI(j)}>
                <img src={x.thumb} alt="" width={72} height={96} loading="lazy" />
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
