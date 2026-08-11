const UPDATE_TIMESTAMP_LABEL_KEY = 'michelangelo/SpecUpdateTimestamp';
const MICROSECONDS_PER_SECOND = 1_000_000;

/**
 * Resolves the epoch-seconds timestamp to display as an entity's "last updated" time.
 *
 * CRD-backed entities record spec updates via a `michelangelo/SpecUpdateTimestamp` metadata label
 * rather than a dedicated status field, because `metadata.creationTimestamp` itself can't be
 * updated once a resource is created. That label is only set once an entity has been updated at
 * least once since the label was introduced, so this falls back to `metadata.creationTimestamp`
 * for rows that predate it or have never been updated.
 *
 * The label is written by the apiserver as microseconds since epoch
 * (`time.Now().UnixMicro()` in `go/api/handler/handler.go`), not seconds, so it must be divided
 * down before being treated as epoch-seconds — otherwise downstream `Date` construction
 * (seconds * 1000) overflows the valid `Date` range and renders as "Invalid date".
 *
 * @example
 * getCrdUpdatedSeconds({
 *   metadata: { labels: { 'michelangelo/SpecUpdateTimestamp': '1700000000000000' } },
 * }) // 1700000000
 *
 * getCrdUpdatedSeconds({
 *   metadata: { creationTimestamp: { seconds: 1650000000 } },
 * }) // 1650000000 (label absent, falls back to creation time)
 */
export function getCrdUpdatedSeconds(data: {
  metadata?: { labels?: Record<string, string>; creationTimestamp?: { seconds: number } };
}): number | undefined {
  const label = data.metadata?.labels?.[UPDATE_TIMESTAMP_LABEL_KEY];
  if (label) return Number(label) / MICROSECONDS_PER_SECOND;
  return data.metadata?.creationTimestamp?.seconds;
}
