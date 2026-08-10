import { CATEGORIES } from '#core/config/categories';
import { ConfigProvider } from '#core/providers/config-provider/config-provider';

import type { StudioConfig } from '#core/types/common/studio-types';
import type { WrapperComponentProps } from './types';

export function getConfigProviderWrapper(config?: StudioConfig) {
  return function ConfigProviderWrapper({ children }: WrapperComponentProps) {
    return (
      <ConfigProvider config={config ?? { categories: CATEGORIES }}>{children}</ConfigProvider>
    );
  };
}
