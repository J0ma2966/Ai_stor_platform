// Деньги приходят в тийинах (1 сум = 100 тийин). Показываем в сумах без копеек.
export function formatSum(tiyin: number, locale: string): string {
  const sum = Math.round(tiyin / 100);
  // Разряды через пробел в обоих языках (так принято и в Узбекистане). Не "uz-UZ": в браузерах без
  // узбекских данных ICU (часть Chromium/WebView) получалось «400,000» и расхождение с сервером.
  // Пробелы неразрывные: сумма не рвётся на строки в узких колонках таблиц и карточек.
  const n = new Intl.NumberFormat("ru-RU").format(sum).replace(/[\s\u202f]/g, "\u00a0");
  return `${n}\u00a0${locale === "uz" ? "so‘m" : "сум"}`;
}

export function formatDateTime(iso: string, locale: string): string {
  // числовой формат дд.мм.гггг, чч:мм одинаков для ru и uz и не зависит от данных ICU в браузере
  void locale;
  return new Intl.DateTimeFormat("ru-RU", {
    day: "2-digit", month: "2-digit", year: "numeric", hour: "2-digit", minute: "2-digit",
    timeZone: "Asia/Tashkent",
  }).format(new Date(iso));
}
