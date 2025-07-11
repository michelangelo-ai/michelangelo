import type { ButtonProps } from 'baseui/button';
import type { ReactNode } from 'react';

export type ErrorViewProps = {
  illustration: ReactNode;
  title: string;
  description?: string | null;
  buttonConfig?: {
    content: ReactNode;
  } & Omit<ButtonProps, 'children'>;
};
