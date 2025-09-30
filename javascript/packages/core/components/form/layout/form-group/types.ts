import type { ReactNode } from 'react';

export interface FormGroupProps {
  title?: string;

  /** Text displayed below title, **Markdown supported** */
  description?: string;

  /** Help tooltip text, displayed next to title. **Markdown supported** */
  tooltip?: string;

  /**
   * Controls whether the group can be collapsed to hide its children
   *
   * @default false
   */
  collapsible?: boolean;

  /** Additional content for the header (e.g., action buttons) */
  endEnhancer?: ReactNode;

  children: ReactNode;
}
