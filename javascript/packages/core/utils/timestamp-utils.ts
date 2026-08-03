const UPDATE_TIMESTAMP_LABEL_KEY = 'michelangelo/SpecUpdateTimestamp';

/**
 * Resolves the epoch-seconds timestamp to display as an entity's "last updated" time.
 *
 * CRD-backed entities record spec updates via a `michelangelo/SpecUpdateTimestamp` metadata label
 * rather than a dedicated status field, because `metadata.creationTimestamp` itself can't be
 * updated once a resource is created. That label is only set once an entity has been updated at
 * least once since the label was introduced, so this falls back to `metadata.creationTimestamp`
 * for rows that predate it or have never been updated.
 *
 * @example
 * getCrdUpdatedSeconds({
 *   metadata: { labels: { 'michelangelo/SpecUpdateTimestamp': '1700000000' } },
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
  if (label) return Number(label);
  return data.metadata?.creationTimestamp?.seconds;
}
