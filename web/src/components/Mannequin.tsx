// Нейтральный манекен в полный рост. Одежду на себя не «надевает» — это ориентир
// для зон (ТЗ 6.1). Тип зависит от пола из профиля.
export function Mannequin({ gender, label }: { gender: "male" | "female" | null; label: string }) {
  const f = gender === "female";
  // плечи/талия/бёдра немного отличаются для мужского и женского типа
  const sh = f ? 34 : 40;
  const wa = f ? 22 : 28;
  const hip = f ? 36 : 31;
  return (
    <svg viewBox="0 0 120 300" role="img" aria-label={label} className="mannequin">
      <g fill="var(--mannequin)" stroke="var(--border)" strokeWidth="1">
        <ellipse cx="60" cy="24" rx="15" ry="18" />
        <rect x="54" y="40" width="12" height="12" rx="4" />
        <path d={`M${60 - sh} 56 Q60 48 ${60 + sh} 56 L${60 + wa} 128 L${60 - wa} 128 Z`} />
        <path d={`M${60 - wa} 128 L${60 + wa} 128 L${60 + hip} 156 L${60 - hip} 156 Z`} />
        <path d={`M${60 - sh} 58 L${60 - sh - 10} 140 L${60 - sh - 3} 142 L${60 - sh + 8} 74 Z`} />
        <path d={`M${60 + sh} 58 L${60 + sh + 10} 140 L${60 + sh + 3} 142 L${60 + sh - 8} 74 Z`} />
        <path d={`M${60 - hip} 156 L${60 - 4} 156 L${60 - 7} 282 L${60 - 19} 282 Z`} />
        <path d={`M${60 + 4} 156 L${60 + hip} 156 L${60 + 19} 282 L${60 + 7} 282 Z`} />
        <ellipse cx="47" cy="288" rx="12" ry="5" />
        <ellipse cx="73" cy="288" rx="12" ry="5" />
      </g>
    </svg>
  );
}
