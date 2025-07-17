import { InterpolationProvider } from '#core/providers/interpolation-provider/interpolation-provider';

import type { ReactNode } from 'react';

interface InterpolationProviderWrapperProps {
  children: ReactNode;
}

export function getInterpolationProviderWrapper(value: Record<string, unknown> = {}) {
  return function InterpolationProviderWrapper({ children }: InterpolationProviderWrapperProps) {
    return <InterpolationProvider value={value}>{children}</InterpolationProvider>;
  };
}
