import type { Override } from 'baseui/overrides';
import type { ReactNode } from 'react';

export type Props = {
  children: ReactNode;
  description?: ReactNode | string;
  overrides?: BoxOverrides;
  title?: ReactNode | string;
};

type BoxOverrides = {
  BoxContainer?: Override;
  BoxDescription?: Override;
  BoxHeader?: Override;
  BoxTitle?: Override;
};
