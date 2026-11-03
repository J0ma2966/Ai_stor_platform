import { headers } from "next/headers";
import { STALE_WORKER_SCRIPT } from "@/lib/staleWorker";

// Снимает чужие service worker до запуска чанков — см. src/lib/staleWorker.ts.
export async function StaleWorkerGuard() {
  const nonce = (await headers()).get("x-nonce") ?? undefined;
  // Браузер скрывает nonce из DOM после загрузки — расхождение атрибута ожидаемо.
  // Текст скрипта — константа без «<», «>» и «&», React выводит его как есть.
  return <script nonce={nonce} suppressHydrationWarning>{STALE_WORKER_SCRIPT}</script>;
}
