import type { ReactNode } from 'react';

export interface FormBannerProps {
  title?: string;

  /**
   * The kind of banner to display
   *
   * @default 'info'
   */
  kind?: 'info' | 'warning';

  /**
   * Controls whether the banner can be dismissed by the user
   *
   * @default false
   */
  dismissible?: boolean;

  /**
   * The content of the banner
   */
  content?: ReactNode | string;
}
