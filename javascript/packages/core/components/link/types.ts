import type { Override } from 'baseui/overrides';
import type { ReactNode } from 'react';

export type LinkProps = {
  children: ReactNode;
  href: string;
  overrides?: LinkOverrides;
  title?: string;
};

type LinkOverrides = {
  Link?: Override;
  ExternalLinkIcon?: Override;
};
