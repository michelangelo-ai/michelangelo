import { AppNavBar } from 'baseui/app-nav-bar';

import { IconProvider } from '#core/providers/icon-provider/icon-provider';
import { ServiceProvider } from '#core/providers/service-provider/service-provider';
import { Router } from '#core/router/router';
import { ThemeProvider } from '#core/themes/provider';

import type { IconProviderContext } from '#core/providers/icon-provider/types';
import type { ServiceContextType } from '#core/providers/service-provider/types';

import '#core/styles/main.css';
// TODO: Relocate the Props interface once the contents of the
// packages/core/index.tsx file are moved to a final location
type Props = {
  dependencies: {
    service: ServiceContextType;
    theme: IconProviderContext;
  };
};

export function CoreApp({ dependencies }: Props) {
  return (
    <ThemeProvider>
      <ServiceProvider {...dependencies.service}>
        <IconProvider icons={dependencies.theme.icons}>
          <AppNavBar title="Michelangelo Studio" />
          <Router />
        </IconProvider>
      </ServiceProvider>
    </ThemeProvider>
  );
}

export { useStudioQuery } from '#core/hooks/use-studio-query';
export { ServiceProvider } from '#core/providers/service-provider/service-provider';

export { getCellRenderer } from '#core/components/cell/get-cell-renderer';
export { BooleanCell } from '#core/components/cell/renderers/boolean/boolean-cell';
export { DateCell } from '#core/components/cell/renderers/date/date-cell';
export { DescriptionCell } from '#core/components/cell/renderers/description/description-cell';
export { DescriptionHierarchy } from '#core/components/cell/renderers/description/constants';

export { DescriptionText } from '#core/components/description-text';
export { TruncatedText } from '#core/components/truncated-text/truncated-text';

export { Icon } from '#core/components/icon/icon';
export { IconKind } from '#core/components/icon/types';
export { IconProvider } from '#core/providers/icon-provider/icon-provider';
