import type { ReactNode } from 'react';
import type { LayoutItem } from '#core/components/form/types/config-types';

export interface FormGridProps {
  children: ReactNode;
}

export type GridLayoutConfig = {
  type: 'grid';
  items: LayoutItem[];
};
