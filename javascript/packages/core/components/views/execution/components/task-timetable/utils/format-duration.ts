export const MINUTE = 60;
export const HOUR = 60 * MINUTE;
export const DAY = 24 * HOUR;

/**
 * Formats a span of seconds as a compact, at-most-two-unit label.
 *
 * @example
 * - formatDuration(12) -> '12s'
 * - formatDuration(105) -> '1m 45s'
 * - formatDuration(7500) -> '2h 5m'
 * - formatDuration(97200) -> '1d 3h'
 */
export function formatDuration(durationSeconds: number): string {
  const duration = Math.max(Math.round(durationSeconds), 0);

  if (duration < MINUTE) return `${duration}s`;
  if (duration < HOUR) return join(duration / MINUTE, 'm', duration % MINUTE, 's');
  if (duration < DAY) return join(duration / HOUR, 'h', (duration % HOUR) / MINUTE, 'm');
  return join(duration / DAY, 'd', (duration % DAY) / HOUR, 'h');
}

function join(value: number, unit: string, remainder: number, remainderUnit: string): string {
  const whole = Math.floor(value);
  const rest = Math.floor(remainder);
  return rest > 0 ? `${whole}${unit} ${rest}${remainderUnit}` : `${whole}${unit}`;
}
