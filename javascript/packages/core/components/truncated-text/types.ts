import type { PopoverOverrides } from 'baseui/tooltip';
import type { ReactNode } from 'react';

export type Props = {
  children: ReactNode | string;
  overrides?: {
    Tooltip: PopoverOverrides;
  };
};
