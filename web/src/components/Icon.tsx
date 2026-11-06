// Набор линейных иконок витрины (вместо эмодзи: одинаково выглядят во всех ОС и красятся цветом текста).
const paths = {
  menu: "M4 6h16M4 12h16M4 18h16",
  search: "M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14zM20 20l-3.5-3.5",
  user: "M12 4a4 4 0 1 0 0 8 4 4 0 0 0 0-8zM4 21c1-4 4-6 8-6s7 2 8 6",
  heart: "M12 20s-7-4.4-7-10a4 4 0 0 1 7-2.6A4 4 0 0 1 19 10c0 5.6-7 10-7 10z",
  bag: "M5 8h14l-1 12H6L5 8zM9 8V6a3 3 0 0 1 6 0v2",
  bell: "M6 16V11a6 6 0 1 1 12 0v5l2 2H4l2-2zM10 20a2 2 0 0 0 4 0",
  globe: "M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM3 12h18M12 3c2.5 2.6 3.8 5.6 3.8 9s-1.3 6.4-3.8 9c-2.5-2.6-3.8-5.6-3.8-9S9.5 5.6 12 3z",
  arrow: "M5 12h14M13 6l6 6-6 6",
  truck: "M3 6h11v10H3zM14 10h4l3 3v3h-7M7 19a2 2 0 1 0 0-4 2 2 0 0 0 0 4zM17 19a2 2 0 1 0 0-4 2 2 0 0 0 0 4z",
  wallet: "M4 7h15a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H4zM4 7l11-3v3M16 13h2",
  store: "M4 9l1.5-5h13L20 9M4 9h16v11H4zM4 9a2.7 2.7 0 0 0 5.3 0 2.7 2.7 0 0 0 5.4 0A2.7 2.7 0 0 0 20 9M10 20v-5h4v5",
  shield: "M12 3l8 3v6c0 4.5-3.4 8-8 9-4.6-1-8-4.5-8-9V6z M9 12l2 2 4-4",
  sparkle: "M12 3l1.8 5.2L19 10l-5.2 1.8L12 17l-1.8-5.2L5 10l5.2-1.8zM19 16l.8 2.2L22 19l-2.2.8L19 22l-.8-2.2L16 19l2.2-.8z",
  headwear: "M4 15a8 8 0 0 1 16 0zM2 15h20v2H2z",
  top: "M9 4l3 2 3-2 5 3-2 4-2-1v10H8V10l-2 1-2-4z",
  bottom: "M7 3h10l1 18h-4l-2-11-2 11H6z",
  shoes: "M3 16v-5l5-1 3 3 9 2v3H3zM3 18h18",
  hanger: "M12 7a2 2 0 1 1 2-2M12 7v2l9 6v2H3v-2l9-6",
  chevronLeft: "M15 5l-7 7 7 7",
  chevronRight: "M9 5l7 7-7 7",
  pause: "M9 5v14M15 5v14",
  play: "M8 5l11 7-11 7z",
  megaphone: "M4 10v4h3l6 4V6L7 10zM16 9a4 4 0 0 1 0 6M7 14l1 5h2l-1-5",
} as const;

export type IconName = keyof typeof paths;

export function Icon({ name, size = 22, className = "", strokeWidth = 1.8, filled = false }: { name: IconName; size?: number; className?: string; strokeWidth?: number; filled?: boolean }) {
  return (
    <svg
      className={`icon ${className}`}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill={filled ? "currentColor" : "none"}
      stroke="currentColor"
      strokeWidth={strokeWidth}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      <path d={paths[name]} />
    </svg>
  );
}
