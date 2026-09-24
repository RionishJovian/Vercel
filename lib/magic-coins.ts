export const MAGIC_COIN_MMK_RATE = 1_000;

export function magicCoinsForMMK(mmk: number): number {
  return mmk / MAGIC_COIN_MMK_RATE;
}

export function fmtMagicCoins(coins: number): string {
  return new Intl.NumberFormat("en-US", { maximumFractionDigits: 2 }).format(coins);
}
