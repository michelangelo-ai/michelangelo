const SPEC_UPDATE_TIMESTAMP_LABEL_KEY = 'michelangelo/SpecUpdateTimestamp';
const UPDATE_TIMESTAMP_LABEL_KEY = 'michelangelo/UpdateTimestamp';
const MICROSECONDS_PER_SECOND = 1_000_000;

/**
 * Naming rules for CRD-backed entities, per Kubernetes RFC 1123 label names
 * (metadata.name): lowercase alphanumerics and '-', starting and ending with
 * an alphanumeric, at most 63 characters.
 */
export const K8S_NAME_MAX_LENGTH = 63;
export const K8S_NAME_PATTERN = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/;
export const K8S_NAME_RULES_MESSAGE =
  "Must only contain lowercase alphanumeric characters, '-', and must start and end with an alphanumeric character";

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
  const label = data.metadata?.labels?.[SPEC_UPDATE_TIMESTAMP_LABEL_KEY];
  if (label) return Number(label) / MICROSECONDS_PER_SECOND;
  return data.metadata?.creationTimestamp?.seconds;
}

/**
 * Resolves the epoch-seconds timestamp to display as an entity's "last updated" time,
 * counting any update — not just a spec change — unlike {@link getCrdUpdatedSeconds}.
 *
 * Some CRD-backed entities (e.g. a pipeline run) have a `status` that changes continuously
 * while their `spec` is effectively immutable after creation. For those, `getCrdUpdatedSeconds`'s
 * spec-only label would render a value frozen at creation time for nearly every row, which
 * defeats the point of a "last updated" column. This instead reads the `michelangelo/UpdateTimestamp`
 * metadata label, which the apiserver refreshes on every update to the resource, with the same
 * `metadata.creationTimestamp` fallback for rows that predate the label or have never been updated.
 *
 * @example
 * getCrdLastUpdatedSeconds({
 *   metadata: { labels: { 'michelangelo/UpdateTimestamp': '1700000000000000' } },
 * }) // 1700000000
 *
 * getCrdLastUpdatedSeconds({
 *   metadata: { creationTimestamp: { seconds: 1650000000 } },
 * }) // 1650000000 (label absent, falls back to creation time)
 */
export function getCrdLastUpdatedSeconds(data: {
  metadata?: { labels?: Record<string, string>; creationTimestamp?: { seconds: number } };
}): number | undefined {
  const label = data.metadata?.labels?.[UPDATE_TIMESTAMP_LABEL_KEY];
  if (label) return Number(label) / MICROSECONDS_PER_SECOND;
  return data.metadata?.creationTimestamp?.seconds;
}
