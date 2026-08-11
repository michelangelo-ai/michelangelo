import type { ReactNode } from 'react';
import type { LayoutItem } from '#core/components/form/types/config-types';

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

export type FormRowLayoutConfig = Pick<FormRowProps, 'name' | 'span'> & {
  type: 'row';
  items: LayoutItem[];
};
