import { ApiError } from "./api";

const TECHNICAL = /^(http-\d+|network-unreachable|unauthorized)$/;

/**
 * The words to show someone when a request or action fails.
 *
 * Only text the API wrote for people is shown. A browser or code error, such as
 * "Cannot read properties of undefined", is never shown: it would tell them
 * nothing they can act on, so they get the fallback instead.
 */
export function errorText(e: unknown, fallback: string): string {
  if (!(e instanceof ApiError)) return fallback;
  const data = e.data as { reason?: unknown; error?: unknown } | null;
  for (const text of [data?.reason, data?.error, e.message]) {
    if (typeof text === "string" && text && !TECHNICAL.test(text)) return text;
  }
  return fallback;
}
