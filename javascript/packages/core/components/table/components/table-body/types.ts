import type { ReactNode } from 'react';
import type { SelectableCapability } from '#core/components/table/types/column-types';
import type { TableData } from '#core/components/table/types/data-types';
import type { WithStickySidesProps } from '../with-sticky-sides/types';

export type TableCell<_T extends TableData = TableData> = {
  id: string;
  content: ReactNode;
};

export type TableRow<T extends TableData = TableData> = {
  id: string;
  cells: TableCell<T>[];
} & SelectableCapability &
  ExpandableCapability;

export type TableBodyProps<T extends TableData = TableData> = {
  rows: TableRow<T>[];
  enableRowSelection: boolean;
  subRow?: React.ComponentType<{ row: TableRow<T> }>;
  actions?: React.ComponentType<{ row: TableRow<T> }>;
} & Pick<WithStickySidesProps, 'enableStickySides' | 'scrollRatio'>;

/**
 * Defines a row's expansion capabilities and current state.
 * Used by expandable row components to enable sub-row expansion interactions.
 *
 * @example
 * ```ts
 * // isExpanded = false
 * onToggleExpanded()
 * expect(isExpanded).toBe(true)
 * ```
 */
export type ExpandableCapability = {
  canExpand: boolean;
  isExpanded: boolean;
  onToggleExpanded: () => void;
};
