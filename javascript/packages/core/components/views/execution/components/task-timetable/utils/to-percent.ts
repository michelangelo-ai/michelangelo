/** Expresses a span as a percentage of the timeline, or 0 when the timeline is empty. */
export function toPercent(valueSeconds: number, totalSeconds: number): number {
  return totalSeconds > 0 ? (valueSeconds / totalSeconds) * 100 : 0;
}
