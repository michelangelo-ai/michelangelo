import type { ReactNode } from 'react';
import type { TableData } from '#core/components/table/types/data-types';

export type TableHeader<_T extends TableData = TableData> = {
  id: string;
  content: ReactNode;
};

export type TableHeaderProps<T extends TableData = TableData> = {
  headers: TableHeader<T>[];
};
