import type { PageSizeOption } from './types';

export enum TablePageSize {
  FIFTEEN = 15,
  TWENTY_FIVE = 25,
  FIFTY = 50,
}

export const DEFAULT_PAGE_SIZE = TablePageSize.FIFTEEN;
export const MIN_PAGE_SIZE = TablePageSize.FIFTEEN;

export const PAGE_SIZE_SELECTION_OPTIONS: PageSizeOption[] = [
  {
    id: MIN_PAGE_SIZE,
    label: String(MIN_PAGE_SIZE),
  },
  {
    id: TablePageSize.TWENTY_FIVE,
    label: String(TablePageSize.TWENTY_FIVE),
  },
  {
    id: TablePageSize.FIFTY,
    label: String(TablePageSize.FIFTY),
  },
];
