import type { ReactNode } from 'react';
import type { ColumnConfig } from '#core/components/table/types/column-types';

export type MainViewContainerProps = {
  children: ReactNode;
  hasBreadcrumb?: boolean;
};

export interface ListViewConfig {
  type: 'list';
  columns: ColumnConfig[];
}

export type ViewConfig = ListViewConfig;
