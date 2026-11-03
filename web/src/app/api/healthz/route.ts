// Проверка живости для балансировщика и Docker (вне next-intl middleware: matcher исключает /api).
export const dynamic = "force-dynamic";

export function GET() {
  return new Response("ok", { headers: { "Cache-Control": "no-store", "Content-Type": "text/plain" } });
}
