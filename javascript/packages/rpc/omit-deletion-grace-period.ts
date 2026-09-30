/**
 * Removes `metadata.deletionGracePeriodSeconds` from every object metadata in a request,
 * including metadata nested inside Any payloads (e.g. Revision content).
 *
 * Envoy prints default values into responses (`always_print_primitive_fields`), and that
 * includes this unset proto2 field as `"0"`. Sending a fetched record back on update would
 * then set it explicitly, which the API server rejects as a change to an immutable field.
 * The server owns this field, so requests never need to carry it.
 */
export function omitDeletionGracePeriod(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(omitDeletionGracePeriod);
  if (value === null || typeof value !== 'object') return value;

  const result: Record<string, unknown> = {};
  for (const [key, val] of Object.entries(value)) {
    result[key] = omitDeletionGracePeriod(val);
  }
  const { metadata } = result;
  if (metadata !== null && typeof metadata === 'object' && !Array.isArray(metadata)) {
    // cast: metadata is a non-null, non-array object
    const { deletionGracePeriodSeconds: _omitted, ...rest } = metadata as Record<string, unknown>;
    result.metadata = rest;
  }
  return result;
}
