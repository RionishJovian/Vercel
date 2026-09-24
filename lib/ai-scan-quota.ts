/** Default monthly manual AI scan allowance (hard monthly reset). */
export const DEFAULT_MONTHLY_AI_SCAN_LIMIT = 30;

/**
 * Legacy compatibility constant. It is no longer selected by application code.
 */
export const PLUS_MONTHLY_AI_SCAN_LIMIT = 50;

export type AiScanQuotaMath = {
  /** Effective ceiling for the current period (month or YTD bank). */
  limit: number;
  used: number;
  remaining: number;
  /** Monthly grant for the active scan window. */
  monthlyGrant: number;
  /** True when unused scans roll into later months until the annual reset. */
  carry: boolean;
};

/**
 * Pure quota math.
 *
 * The live application uses the hard monthly-cap branch. The optional legacy
 * flag remains only so historic analytics can be decoded safely.
 */
export function computeAiScanQuota(input: {
  plus: boolean;
  monthlyGrant: number;
  /** Scans already consumed in the active window (month or year). */
  used: number;
  /** 1–12 month index in app time. */
  monthIndex: number;
}): AiScanQuotaMath {
  const grant = Math.max(0, Math.floor(input.monthlyGrant));
  const used = Math.max(0, Math.floor(input.used));
  if (!input.plus) {
    const limit = grant;
    return {
      limit,
      used,
      remaining: Math.max(0, limit - used),
      monthlyGrant: grant,
      carry: false,
    };
  }
  const monthsElapsed = Math.min(12, Math.max(1, Math.floor(input.monthIndex)));
  const limit = monthsElapsed * grant;
  return {
    limit,
    used,
    remaining: Math.max(0, limit - used),
    monthlyGrant: grant,
    carry: true,
  };
}
