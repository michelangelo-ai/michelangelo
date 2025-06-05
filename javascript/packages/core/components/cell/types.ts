import type { Theme } from 'baseui';
import type { ReactNode } from 'react';
import type { StyleObject } from 'styletron-react';
import type { DescriptionCellConfig } from '#core/components/cell/renderers/description/types';
import type { LinkCellConfig } from '#core/components/cell/renderers/link/types';
import type { MultiCellConfig } from '#core/components/cell/renderers/multi/types';
import type { Accessor } from '#core/types/common/studio-types';
import type { StateCellConfig } from './renderers/state/types';
import type { TypeCellConfig } from './renderers/type/types';

/**
 * @description
 * A union type of all cell configurations. This type extends the {@link SharedCell} type
 * with cell configurations for the different cell renderers. For example, the {@link DescriptionCellConfig}
 * type extends the {@link SharedCell} type with a `hierarchy` property.
 *
 * @see {@link DescriptionCellConfig}
 * @see {@link LinkCellConfig}
 * @see {@link MultiCellConfig}
 */
export type Cell = SharedCell &
  (DescriptionCellConfig | LinkCellConfig | MultiCellConfig | StateCellConfig | TypeCellConfig);

export interface SharedCell<T = unknown> {
  /**
   * @description Unique identifier for the column
   * If no accessor is provided, this id will be used to access the data
   * @example 'metadata.name'
   */
  id: string;

  /**
   * @description Used to more flexibly control the cell value by providing a custom json-path or a function
   * @example 'spec.content.metadata.name'
   * @example (row) => `Revision ${row?.spec?.revisionId}`,
   */
  accessor?: Accessor<T>;

  /**
   * @description Label to be displayed in the table header
   */
  label?: string;

  /**
   * @description Helper field that can control Filter type therefore the Filter UI (simple select/ date picker)
   * @example ColumnType.DATE
   */
  type?: string;

  /**
   * @description Icon to be displayed in the data cell before the value
   */
  icon?: string;

  /**
   * @description When provided, the cell will display a tooltip on hover
   */
  tooltip?: CellTooltip;

  /**
   * @description Decorates every cell in the column row with content
   *
   * @example
   * {
   *  type: 'tooltip',
   *  content: 'Show this tooltip next to the cell
   * }
   */
  endEnhancer?: {
    content: ReactNode;
    type: 'tooltip';
  };

  /**
   * @description Custom cell renderer
   *
   * @remarks Use caution when leveraging a custom `Cell` renderer, as the default
   * renderer provides styling/hyperlinking/etc.
   *
   * Either ensure the cell you are configuring:
   *  - Does not need this functionality
   *  - Applies its own copy of this functionality
   *
   * @default
   * @see src/components/cell/renderers/default/column-renderer.tsx
   */
  Cell?: CellRenderer<T>;

  /**
   * @description Style overrides to be applied to each cell
   *
   * @default {}
   * @see src/components/pages/studio-evaluation-charts/report-charts/report-table/utils/columns-builder.ts
   */
  style?: StyleObject | CellStyleFunction;
}

export type CellTooltip = {
  content: string | ((params: CellRendererProps) => React.ReactNode);
  action: 'filter' | 'custom';
};

/**
 * A function type that defines the style for a cell based on the provided arguments.
 *
 * @param args - An object containing the following properties:
 *   @property record - The data record associated with the cell.
 *   @property theme - The theme object used for styling.
 *
 * @returns A `StyleObject` that represents the computed style for the cell.
 */
export type CellStyleFunction = (args: { record: unknown; theme: Theme }) => StyleObject;

export type CellRenderer<T, CellConfig = SharedCell<T>> = {
  (props: CellRendererProps<T, CellConfig>): ReactNode | null;

  /**
   * @description
   * Primary consumer is table filtering smart search
   */
  toString?: (props: CellRendererProps<T, CellConfig>) => string;
};

export interface CellRendererProps<T = unknown, CellConfig = SharedCell<T>> {
  column: CellConfig;

  /**
   * @description
   * The record of data for the cell
   *
   * @remarks
   * The value of the cell is the value of the record at the path specified by the
   * accessor.
   */
  record: object;

  /**
   * @description
   * This is the value that will be displayed in the cell.
   *
   * @remarks
   * This is the value that will be displayed in the cell.
   */
  value: T | undefined;

  /**
   * @description
   * The central Cell rendering component. This ensures that any cell renderers
   * that require recursive rendering can access functionality provided by the
   * central Cell renderer. For example, context, like table filtering, that is
   * only available to a the Table instantiation of Cells
   *
   * @see
   * src/components/cell/renderers/switch-type-meta
   *
   * @default
   * src/components/cell/renderers/default/column-renderer.tsx
   */
  CellComponent?: CellRenderer<T>;
}

export type CellToStringParams<T = unknown, CellConfig = SharedCell> = Pick<
  CellRendererProps<T, CellConfig>,
  'column' | 'value'
>;
