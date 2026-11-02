/** Speed limit for a guest plan, in megabits per second. Both numbers are set or neither is (0 = no limit). */
export interface PlanSpeed { speed_down_mbps?: number; speed_up_mbps?: number }

export const MAX_SPEED_MBPS = 10000;

/** True when the pair is something the server accepts: both 0, or both a whole number from 1 to 10000. */
export function speedOk(p: PlanSpeed): boolean {
  const d = p.speed_down_mbps ?? 0, u = p.speed_up_mbps ?? 0;
  if (!Number.isInteger(d) || !Number.isInteger(u)) return false;
  if (d === 0 && u === 0) return true;
  return d >= 1 && d <= MAX_SPEED_MBPS && u >= 1 && u <= MAX_SPEED_MBPS;
}

/** Whole-number reading of a number input; blank or junk is 0, never NaN. */
export const speedValue = (raw: string): number => {
  const n = parseInt(raw || "0", 10);
  return Number.isFinite(n) && n > 0 ? n : 0;
};
