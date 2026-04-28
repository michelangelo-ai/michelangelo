import type { ReactNode } from 'react';

export interface FormBannerProps {
  title?: string;

  /** @default 'info' */
  kind?: 'info' | 'warning';

  /**
   * Once dismissed, the banner stays hidden for the component's lifetime.
   *
   * @default false
   */
  dismissible?: boolean;

  /** String values are rendered as Markdown. */
  content?: ReactNode | string;
}
