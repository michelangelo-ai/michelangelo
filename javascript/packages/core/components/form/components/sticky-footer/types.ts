import type { ReactNode } from 'react';

export interface StickyFooterProps {
  /** Usually form actions (e.g., submit button). */
  rightContent?: ReactNode;

  /** Usually secondary info, status text. */
  leftContent?: ReactNode;
}
