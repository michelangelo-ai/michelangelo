import type { ActionConfig, Data } from '#core/components/actions/types';
import type { PartitionedActions } from './types';

/**
 * Splits an actions array by hierarchy level. Actions without an explicit
 * hierarchy default to tertiary (overflow menu).
 */
export function partitionActions<T extends Data>(
  actions: ActionConfig<T>[]
): PartitionedActions<T> {
  return {
    primary: actions.find((a) => a.hierarchy === 'primary'),
    secondary: actions.filter((a) => a.hierarchy === 'secondary'),
    tertiary: actions.filter((a) => !a.hierarchy || a.hierarchy === 'tertiary'),
  };
}
