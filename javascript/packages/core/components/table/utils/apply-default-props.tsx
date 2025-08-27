import { CircleExclamationMark } from '#core/components/illustrations/circle-exclamation-mark/circle-exclamation-mark';
import { CircleExclamationMarkKind } from '#core/components/illustrations/circle-exclamation-mark/types';
import { TableBody } from '../components/table-body/table-body';
import { TableLoadingState } from '../components/table-loading-state';
import { PAGE_SIZE_SELECTION_OPTIONS } from '../components/table-pagination/constants';
import { TablePagination } from '../components/table-pagination/table-pagination';
import { normalizePageSize } from '../components/table-pagination/utils';

import type { PageSizeOption } from '../components/table-pagination/types';
import type { TableData } from '../types/data-types';
import type { ControlledTableState, TableProps, TablePropsResolved } from '../types/table-types';

/**
 * Applies default properties to the given table properties.
 *
 * This function merges the provided table properties with a set of default
 * properties to ensure that all necessary properties are defined.
 */
export function applyDefaultProps<T extends TableData = TableData>(
  props: TableProps<T>
): TablePropsResolved<T> {
  const disablePagination = props.disablePagination ?? false;
  const disableSorting = props.disableSorting ?? false;
  const pageSizes = props.pageSizes ?? PAGE_SIZE_SELECTION_OPTIONS;

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
    error: props.error ?? undefined,
    actionBarConfig: {
      enableSearch: true,
      enableFilters: true,
      ...props.actionBarConfig,
    },
    disablePagination,
    disableSorting,
    pageSizes,
    state: resolveTableState(props.state, disablePagination, pageSizes),
    pagination: props.pagination ?? TablePagination,
    enableRowSelection: props.enableRowSelection ?? false,
    enableStickySides: props.enableStickySides ?? false,
    body: props.body ?? TableBody,
  };
}

function resolveTableState(
  userState: Partial<ControlledTableState> | undefined,
  disablePagination: boolean,
  pageSizes: PageSizeOption[]
): Partial<ControlledTableState> | undefined {
  if (disablePagination || userState?.setPagination) {
    return userState;
  }

  const requestedPageSize = userState?.pagination?.pageSize;
  const normalizedPageSize = normalizePageSize(requestedPageSize, pageSizes);

  return {
    ...userState,
    pagination: {
      pageIndex: 0,
      ...userState?.pagination,
      pageSize: normalizedPageSize,
    },
  };
}
