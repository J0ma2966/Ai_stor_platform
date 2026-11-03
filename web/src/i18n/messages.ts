// Тексты админки лежат в отдельных файлах (messages/admin.<locale>.json), чтобы
// не мешать правкам общих переводов. Здесь они сливаются с основными по пространствам имён.
type Messages = Record<string, Record<string, unknown>>;

export function mergeMessages(base: Messages, extra: Messages): Messages {
  const out: Messages = { ...base };
  for (const [ns, v] of Object.entries(extra)) out[ns] = { ...(base[ns] ?? {}), ...v };
  return out;
}
