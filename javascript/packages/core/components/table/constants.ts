import type { TableState } from './types/table-types';

export const TABLE_STATE_DEFAULTS: TableState = {
  globalFilter: '',
} as const;

export const TABLE_LOCAL_STORAGE_KEY = 'ma-studio-table-settings';
