// Единый API-клиент.
// - access-токен живёт только в памяти (не localStorage): XSS не может его вытащить из хранилища;
// - refresh-токен — httpOnly cookie на домене API (credentials: "include");
// - на 401 token_invalid один раз обновляем токен и повторяем запрос;
// - на token_revoked / refresh_invalid — выход и переход на экран входа.

export const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

export type Role = "client" | "store" | "admin";

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    public fields: Record<string, string> = {},
    public requestId = "",
  ) {
    super(message);
  }
}

let accessToken: string | null = null;
let role: Role | null = null;
let onAuthLost: (() => void) | null = null;

export function setSession(token: string | null, r: Role | null = null) {
  accessToken = token;
  role = r;
}
export function currentRole(): Role | null {
  return role;
}
export function setOnAuthLost(fn: (() => void) | null) {
  onAuthLost = fn;
}

type SessionResp = { access_token: string; user: { id: string; role: Role }; is_new_user?: boolean };

// Результат refresh. Без error — сессии нет (cookie нет или она недействительна).
// С error — временный сбой (429 по лимиту, 5xx, нет сети): cookie при этом остаётся
// рабочей, поэтому выходить из аккаунта и отправлять на экран входа нельзя.
export type RefreshResult = { ok: true } | { ok: false; error?: ApiError };

// refresh: параллельные запросы делят один вызов /auth/refresh.
let refreshing: Promise<RefreshResult> | null = null;

export function refreshSessionResult(): Promise<RefreshResult> {
  if (!refreshing) {
    refreshing = (async (): Promise<RefreshResult> => {
      try {
        const res = await fetch(`${API_URL}/v1/auth/refresh`, { method: "POST", credentials: "include" });
        if (!res.ok) {
          if (res.status === 401 || res.status === 403) {
            setSession(null);
            return { ok: false };
          }
          return { ok: false, error: await parseError(res) };
        }
        const body = (await res.json()) as { data: SessionResp };
        setSession(body.data.access_token, body.data.user.role);
        return { ok: true };
      } catch {
        return { ok: false, error: new ApiError(0, "network", "network error") };
      } finally {
        refreshing = null;
      }
    })();
  }
  return refreshing;
}

export function refreshSession(): Promise<boolean> {
  return refreshSessionResult().then((r) => r.ok);
}

export type RequestOpts = {
  method?: string;
  body?: unknown;
  headers?: Record<string, string>;
  rawBody?: Blob;
  signal?: AbortSignal;
};

async function parseError(res: Response): Promise<ApiError> {
  let code = "internal";
  let message = res.statusText;
  let fields: Record<string, string> = {};
  let requestId = res.headers.get("X-Request-ID") ?? "";
  try {
    const b = await res.json();
    code = b?.error?.code ?? code;
    message = b?.error?.message ?? message;
    fields = b?.error?.fields ?? {};
    requestId = b?.request_id ?? requestId;
  } catch {
    /* не JSON — оставляем статус */
  }
  return new ApiError(res.status, code, message, fields, requestId);
}

export async function apiRaw(path: string, opts: RequestOpts = {}, retried = false): Promise<Response> {
  const headers: Record<string, string> = { ...(opts.headers ?? {}) };
  if (opts.body !== undefined) headers["Content-Type"] = "application/json";
  if (accessToken) headers["Authorization"] = `Bearer ${accessToken}`;
  let res: Response;
  try {
    res = await fetch(`${API_URL}${path}`, {
      method: opts.method ?? "GET",
      headers,
      body: opts.rawBody ?? (opts.body !== undefined ? JSON.stringify(opts.body) : undefined),
      credentials: "include",
      signal: opts.signal,
    });
  } catch (e) {
    if ((e as Error).name === "AbortError") throw e;
    throw new ApiError(0, "network", "network error");
  }
  if (res.ok) return res;
  const err = await parseError(res);
  if (res.status === 401 && !retried && (err.code === "token_invalid" || (err.code === "unauthorized" && !accessToken))) {
    if (await refreshSession()) return apiRaw(path, opts, true);
  }
  if (err.code === "token_revoked" || err.code === "refresh_invalid" || (res.status === 401 && retried)) {
    setSession(null);
    onAuthLost?.();
  }
  throw err;
}

export async function api<T>(path: string, opts: RequestOpts = {}): Promise<T> {
  const res = await apiRaw(path, opts);
  if (res.status === 204) return undefined as T;
  const body = await res.json();
  return body.data as T;
}

export async function apiList<T>(path: string): Promise<{ items: T[]; nextCursor: string }> {
  const res = await apiRaw(path);
  const body = await res.json();
  return { items: body.data as T[], nextCursor: body.next_cursor ?? "" };
}

// ---- вход ----
export async function requestOtp(phone: string) {
  await api("/v1/auth/otp/request", { method: "POST", body: { phone } });
}
export async function verifyOtp(phone: string, code: string) {
  const s = await api<SessionResp>("/v1/auth/otp/verify", { method: "POST", body: { phone, code } });
  setSession(s.access_token, s.user.role);
  return s;
}
// Пароль клиента: обычный вход без SMS; SMS нужна только для регистрации и сброса.
export async function loginPassword(phone: string, password: string) {
  const s = await api<SessionResp>("/v1/auth/password/login", { method: "POST", body: { phone, password } });
  setSession(s.access_token, s.user.role);
  return s;
}
export async function resetPassword(phone: string, code: string, password: string) {
  const s = await api<SessionResp>("/v1/auth/password/reset", { method: "POST", body: { phone, code, password } });
  setSession(s.access_token, s.user.role);
  return s;
}
// Задать первый пароль или сменить (тогда нужен текущий). Сервер выдаёт новую сессию
// и отзывает остальные устройства.
export async function setPassword(password: string, currentPassword?: string) {
  const s = await api<SessionResp>("/v1/auth/password", {
    method: "POST",
    body: { password, current_password: currentPassword || undefined },
  });
  setSession(s.access_token, s.user.role);
  return s;
}
export async function loginStaff(login: string, password: string) {
  const s = await api<SessionResp>("/v1/auth/login", { method: "POST", body: { login, password } });
  setSession(s.access_token, s.user.role);
  return s;
}
export async function logout() {
  try {
    await api("/v1/auth/logout", { method: "POST" });
  } finally {
    setSession(null);
  }
}

export function newIdempotencyKey(): string {
  return crypto.randomUUID();
}
