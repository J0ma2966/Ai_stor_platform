import type { NextRequest } from "next/server";
import createMiddleware from "next-intl/middleware";
import { routing } from "./i18n/routing";
import { buildCsp, newNonce } from "./lib/csp";

const intl = createMiddleware(routing);

export default function middleware(req: NextRequest) {
  const nonce = newNonce();
  const csp = buildCsp(nonce);
  // Next читает nonce из CSP-заголовка запроса и ставит его на свои <script>.
  // next-intl копирует заголовки запроса в next()/rewrite(), поэтому меняем их до вызова.
  req.headers.set("x-nonce", nonce);
  req.headers.set("Content-Security-Policy", csp);
  const res = intl(req);
  res.headers.set("Content-Security-Policy", csp);
  return res;
}

export const config = {
  matcher: [{ source: "/((?!api|_next/static|_next/image|favicon.ico|.*\\..*).*)", missing: [{ type: "header", key: "next-router-prefetch" }] }],
};
