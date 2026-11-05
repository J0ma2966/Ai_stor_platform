"use client";
import { useId, useState } from "react";
import { MAX_RATING, MIN_RATING } from "@/lib/reviews";

// Звёзды оценки только для показа. label — текст для экранных дикторов («Оценка 4 из 5»).
export function Stars({ value, label, size = "sm" }: { value: number; label: string; size?: "sm" | "lg" }) {
  const pct = Math.max(0, Math.min(100, (value / MAX_RATING) * 100));
  return (
    <span className={`stars stars-${size}`} role="img" aria-label={label}>
      <span className="stars-bg" aria-hidden="true">★★★★★</span>
      <span className="stars-fg" aria-hidden="true" style={{ width: `${pct}%` }}>★★★★★</span>
    </span>
  );
}

// Выбор оценки: радиокнопки (работают с клавиатуры стрелками), оформленные звёздами.
export function StarInput({ value, onChange, legend, labelFor }: { value: number; onChange: (v: number) => void; legend: string; labelFor: (n: number) => string }) {
  const name = useId();
  const [hover, setHover] = useState(0);
  const shown = hover || value;
  const options = Array.from({ length: MAX_RATING - MIN_RATING + 1 }, (_, i) => MIN_RATING + i);
  return (
    <fieldset className="star-input" onMouseLeave={() => setHover(0)}>
      <legend>{legend}</legend>
      {options.map((n) => (
        <label key={n} className={n <= shown ? "on" : undefined} onMouseEnter={() => setHover(n)}>
          <input type="radio" name={name} value={n} checked={value === n} onChange={() => onChange(n)} />
          <span aria-hidden="true">★</span>
          <span className="sr-only">{labelFor(n)}</span>
        </label>
      ))}
    </fieldset>
  );
}
