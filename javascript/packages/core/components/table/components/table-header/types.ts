import type {
  ColumnRenderState,
  SelectableCapability,
  SortingCapability,
  VisibilityCapability,
} from '#core/components/table/types/column-types';
import type { TableData } from '#core/components/table/types/data-types';
import type { ControlledTableState } from '#core/components/table/types/table-types';
import type { WithStickySidesProps } from '../with-sticky-sides/types';

export type TableHeaderProps<T extends TableData = TableData> = {
  columns: Array<ColumnRenderState<T> & SortingCapability & VisibilityCapability>;
  setColumnOrder: ControlledTableState['setColumnOrder'];
  setColumnVisibility: ControlledTableState['setColumnVisibility'];
  enableRowSelection: boolean;
  /**
   * Pins the header row to the top of the table's scroll container. Set by Table when
   * `maxHeight` caps the container, so the header stays visible while rows scroll beneath it.
   */
  stickyHeader?: boolean;
} & Omit<SelectableCapability, 'canSelect'> &
  Pick<WithStickySidesProps, 'enableStickySides' | 'scrollRatio'>;
