import type { ReactNode } from 'react';
import type { Cell } from '#core/components/cell/types';
import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { DetailPageConfig } from '#core/components/views/detail-view/types/detail-view-schema-types';

export type MainViewContainerProps = {
  children: ReactNode;
  hasBreadcrumb?: boolean;
};

export type ViewConfig = ListViewConfig | DetailViewConfig;

export interface ListViewConfig {
  type: 'list';
  columns: ColumnConfig[];
}

export interface DetailViewConfig<T extends object = object> {
  type: 'detail';

  /** Metadata items to display in the detail view header */
  metadata: Cell[];

  /** Content sections to display in the detail view */
  pages: DetailPageConfig<T>[];
}
