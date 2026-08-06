import type { ReactNode } from 'react';

export interface FormRowProps {
  /** Optional row label */
  name?: string;

  /**
   * Column spans for each child element
   *
   * Defaults to equal spacing for all children
   */
  span?: number[];
  children: ReactNode;
}

/** Declarative config fields for a row layout — excludes children. */
export type RowLayoutConfigFields = {
  name?: string;
  span?: number[];
};
