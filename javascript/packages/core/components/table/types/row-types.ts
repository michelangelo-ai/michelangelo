import type { ReactNode } from 'react';
import type { ColumnConfig } from './column-types';
import type { SelectableCapability } from './column-types';
import type { TableData } from './data-types';

export type TableCell<T extends TableData = TableData> = {
  id: string;
  content: ReactNode;
  column: ColumnConfig<T>;
  value: unknown;
  isVisible: boolean;
};

export type TableRow<T extends TableData = TableData> = {
  id: string;
  cells: TableCell<T>[];
  record: T;
} & SelectableCapability &
  ExpandableCapability;

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
