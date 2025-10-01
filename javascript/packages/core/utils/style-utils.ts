import { mergeOverrides } from 'baseui';

import type { Overrides } from 'baseui/overrides';

/**
 * Merges multiple override objects into a single override object.
 * Internally leverages baseui's {@link mergeOverrides} function.
 *
 * @remarks
 * Use this when combining 3+ overrides to avoid nested `mergeOverrides` calls.
 * For merging just 2 overrides, use baseui's `mergeOverrides` directly.
 *
 * @example
 * ```tsx
 * // Without mergeAllOverrides (nested, harder to read)
 * mergeOverrides(
 *   mergeOverrides(
 *     mergeOverrides(overrides, headerDefaults),
 *     buttonDefaults
 *   ),
 *   scrollDefaults
 * )
 *
 * // With mergeAllOverrides (flat, easier to read)
 * mergeAllOverrides(overrides, headerDefaults, buttonDefaults, scrollDefaults)
 * ```
 */
export function mergeAllOverrides(...overrides: Array<Overrides>): Overrides {
  return overrides.reduce((result, current) => {
    return mergeOverrides(result, current);
  }, {});
}
