import type { ReactNode } from 'react';
import type { Cell } from '#core/components/cell/types';
import type { ColumnConfig } from '#core/components/table/types/column-types';

export type MainViewContainerProps = {
  children: ReactNode;
  hasBreadcrumb?: boolean;
};

export type ViewConfig = ListViewConfig | DetailViewConfig;

export interface ListViewConfig {
  type: 'list';
  columns: ColumnConfig[];
}

export interface DetailViewConfig {
  type: 'detail';
  /**
   * Metadata items to display in the detail view header
   */
  metadata: Cell[];
  /**
   * Content sections to display in the detail view
   */
  pages: DetailPageConfig[];
}

export interface DetailPageConfig {
  /**
   * Type of page content to render
   */
  type: string;
}
