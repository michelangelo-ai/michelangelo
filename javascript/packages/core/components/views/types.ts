import type { ReactNode } from 'react';
import type { Cell } from '#core/components/cell/types';
import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { DetailPageConfig } from '#core/components/views/detail-view/types/detail-view-schema-types';

export type MainViewContainerProps = {
  children: ReactNode;
  hasBreadcrumb?: boolean;
};

export type ViewConfig<T extends object = object> = ListViewConfig<T> | DetailViewConfig<T>;

export interface ListViewConfig<T extends object = object> {
  type: 'list';
  columns: ColumnConfig<T>[];
}

export interface DetailViewConfig<T extends object = object> {
  type: 'detail';

  /** Metadata items to display in the detail view header */
  metadata: Cell[];

  /** Content sections to display in the detail view */
  pages: DetailPageConfig<T>[];
}
