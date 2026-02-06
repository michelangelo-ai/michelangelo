import { useStyletron } from 'baseui';

import type { StyleObject } from 'styletron-react';
import type { SharedCell } from './types';

/**
 * Resolves cell styles from either static style objects or dynamic style functions.
 *
 * This hook enables cells to have conditional styling based on the record data.
 * When a function is provided, it receives the current record and theme, allowing
 * for row-specific styling decisions.
 *
 * @param args.record - The data record for the current row
 * @param args.style - Either a static StyleObject or a function that returns styles
 *   based on the record and theme. Undefined is treated as no styles.
 *
 * @returns Resolved StyleObject to be applied to the cell
 *
 * @example
 * ```typescript
 * // Static styles
 * const styles = useCellStyles({
 *   record: myData,
 *   style: { color: 'blue', fontWeight: 'bold' }
 * });
 * // Returns: { color: 'blue', fontWeight: 'bold' }
 *
 * // Dynamic styles based on record data
 * const styles = useCellStyles({
 *   record: { status: 'error', value: 100 },
 *   style: ({ record, theme }) => ({
 *     color: record.status === 'error'
 *       ? theme.colors.negative
 *       : theme.colors.primary,
 *     fontWeight: record.value > 50 ? 'bold' : 'normal'
 *   })
 * });
 * // Returns: { color: theme.colors.negative, fontWeight: 'bold' }
 *
 * // No styles
 * const styles = useCellStyles({
 *   record: myData,
 *   style: undefined
 * });
 * // Returns: {}
 * ```
 */
export function useCellStyles({
  record,
  style,
}: {
  record: unknown;
  style: SharedCell['style'] | undefined;
}): StyleObject {
  const [, theme] = useStyletron();

  if (!style) return {};

  if (typeof style !== 'function') {
    return style;
  }

  return style({ record, theme });
}
