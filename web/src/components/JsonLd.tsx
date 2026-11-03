import { headers } from "next/headers";

// Структурированные данные для поисковиков. React выводит текст <script> как есть,
// поэтому «<», «>» и «&» экранируются в JSON (\u003c…) — закрыть тег из данных нельзя.
// Скрипт получает nonce из CSP.
export function ldJson(data: unknown): string {
  return JSON.stringify(data).replace(/</g, "\\u003c").replace(/>/g, "\\u003e").replace(/&/g, "\\u0026");
}

export async function JsonLd({ data }: { data: unknown }) {
  const nonce = (await headers()).get("x-nonce") ?? undefined;
  return <script type="application/ld+json" nonce={nonce}>{ldJson(data)}</script>;
}
