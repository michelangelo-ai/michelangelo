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
