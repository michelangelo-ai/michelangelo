import type { ReactNode } from 'react';

export interface LabelProps {
  label: ReactNode;
  required?: boolean;
  description?: string;
}
