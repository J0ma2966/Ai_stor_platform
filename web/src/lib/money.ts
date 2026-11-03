// Деньги приходят в тийинах (1 сум = 100 тийин). Показываем в сумах без копеек.
export function formatSum(tiyin: number, locale: string): string {
  const sum = Math.round(tiyin / 100);
  // разделители — неразрывные пробелы: сумма не рвётся на строки в узких колонках таблиц и карточек
  const n = new Intl.NumberFormat(locale === "uz" ? "uz-UZ" : "ru-RU").format(sum).replace(/[\s\u202f]/g, "\u00a0");
  return `${n}\u00a0${locale === "uz" ? "so‘m" : "сум"}`;
}

export function formatDateTime(iso: string, locale: string): string {
  return new Intl.DateTimeFormat(locale === "uz" ? "uz-UZ" : "ru-RU", {
    day: "2-digit", month: "2-digit", year: "numeric", hour: "2-digit", minute: "2-digit",
    timeZone: "Asia/Tashkent",
  }).format(new Date(iso));
}
