import { RepeatedLayoutProvider } from '#core/providers/repeated-layout-provider/repeated-layout-provider';

import type { ReactNode } from 'react';
import type { RepeatedLayoutState } from '#core/providers/repeated-layout-provider/types';

interface RepeatedLayoutProviderWrapperProps {
  children: ReactNode;
}

export function getRepeatedLayoutProviderWrapper(state: RepeatedLayoutState) {
  return function RepeatedLayoutProviderWrapper({ children }: RepeatedLayoutProviderWrapperProps) {
    return <RepeatedLayoutProvider {...state}>{children}</RepeatedLayoutProvider>;
  };
}
