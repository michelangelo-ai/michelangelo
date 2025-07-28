import { CircleExclamationMark } from '#core/components/illustrations/circle-exclamation-mark/circle-exclamation-mark';
import { CircleExclamationMarkKind } from '#core/components/illustrations/circle-exclamation-mark/types';
import { TableLoadingState } from '../components/table-loading-state';

import type { TableData } from '../types/data-types';
import type { TableProps, TablePropsResolved } from '../types/table-types';

/**
 * Applies default properties to the given table properties.
 *
 * This function merges the provided table properties with a set of default
 * properties to ensure that all necessary properties are defined.
 */
export function applyDefaultProps<T extends TableData = TableData>(
  props: TableProps<T>
): TablePropsResolved<T> {
  return {
    ...props,
    emptyState: props.emptyState ?? {
      icon: (
        <CircleExclamationMark
          kind={CircleExclamationMarkKind.PRIMARY}
          height="64px"
          width="64px"
        />
      ),
      title: 'No data',
      content: 'No data is present.',
    },
    loading: props.loading ?? false,
    loadingView: props.loadingView ?? TableLoadingState,
  };
}
