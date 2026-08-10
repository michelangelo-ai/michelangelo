import type { Override } from 'baseui/overrides';
import type { ReactNode } from 'react';

export type LinkProps = {
  children: ReactNode;
  href: string;
  overrides?: LinkOverrides;
  title?: string;
  /**
   * Invoked when the link is clicked, before navigation occurs. Optional — omit for a plain
   * link with no side effects.
   */
  onClick?: () => void;
};

type LinkOverrides = {
  Link?: Override;
  ExternalLinkIcon?: Override;
};
