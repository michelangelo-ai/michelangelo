import type { ReactNode } from 'react';

export interface FormGridProps {
  children: ReactNode;
}

/** Declarative config fields for a grid layout — excludes children. */
export type GridLayoutConfigFields = Record<string, never>;
