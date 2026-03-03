import type { ReactNode } from 'react';

export interface FormStepProps {
  name: string;
  /** Rendered as Markdown. */
  description?: string;
  children: ReactNode;
}
