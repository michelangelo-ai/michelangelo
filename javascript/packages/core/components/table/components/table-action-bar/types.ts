import type { ReactNode } from 'react';

export interface TableActionBarProps {
  globalFilter: string;
  setGlobalFilter: (value: string) => void;
  configuration: TableActionBarConfig;
}

/**
 * Configuration options for the Table Action Bar component.
 */
export interface TableActionBarConfig {
  /**
   * Indicates whether search functionality is enabled.
   *
   * @default true
   */
  enableSearch?: boolean;

  /**
   * ReactNode to be rendered in the middle section of the action bar.
   */
  middle?: ReactNode;

  /**
   * ReactNode to be rendered in the trailing section of the action bar.
   */
  trailing?: ReactNode;
}
