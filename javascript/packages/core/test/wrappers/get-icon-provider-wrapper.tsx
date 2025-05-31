import { Check } from 'baseui/icon';

import { IconProvider } from '#core/providers/icon-provider/icon-provider';
import { WrapperComponentProps } from './types';

export function getIconProviderWrapper({
  icons = {
    check: Check,
  },
}: { icons?: Record<string, React.ComponentType> } = {}) {
  return function IconProviderWrapper({ children }: WrapperComponentProps) {
    return <IconProvider icons={icons}>{children}</IconProvider>;
  };
}
