import { isRecord } from '#core/utils/object-utils';

/**
 * JSON.stringify with deterministic key ordering. Produces identical output
 * regardless of property insertion order, so `{ a: 1, b: 2 }` and
 * `{ b: 2, a: 1 }` both yield `'{"a":1,"b":2}'`. Handles nested objects.
 */
export function serializeKey(value: unknown): string {
  return JSON.stringify(value, (_, val: unknown) => {
    if (isRecord(val)) {
      return Object.fromEntries(Object.entries(val).sort(([a], [b]) => a.localeCompare(b)));
    }
    return val;
  });
}
