import type { TableState } from '#core/components/table/types/table-types';

export type UrlScope =
  | 'globalFilter'
  | 'columnFilters'
  | 'sorting'
  | 'columnVisibility'
  | 'columnOrder';

export type UrlFiltersConfig = {
  enabled?: boolean;
  scope?: UrlScope[];
  paramPrefix?: string;
};

export type RawUrlTableState = {
  gf?: string;
  cf?: string;
  so?: string;
  cv?: string;
  co?: string;
};

export type UseUrlTableStateOptions = {
  tableSettingsId: string;
  validColumnIds: string[];
  scope?: UrlScope[];
  paramPrefix?: string;
};

export type UseUrlTableStateResult = {
  urlState: Partial<TableState> | null;
  hasUrlState: boolean;
  buildShareUrl: (currentState: Partial<TableState>) => string;
};
