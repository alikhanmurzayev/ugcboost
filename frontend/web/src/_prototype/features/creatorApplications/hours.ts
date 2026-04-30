export function hoursSince(iso: string): number {
  const diffMs = Date.now() - new Date(iso).getTime();
  return Math.max(0, diffMs / (1000 * 60 * 60));
}
