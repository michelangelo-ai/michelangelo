import { AppNavBar } from 'baseui/app-nav-bar';

import { ServiceProvider } from '#core/providers/service-provider/service-provider';
import { Router } from '#core/router/router';
import { ThemeProvider } from '#core/themes/provider';

import type { ServiceContextType } from '#core/providers/service-provider/types';

import '#core/styles/main.css';

// TODO: Relocate the Props interface once the contents of the
// packages/core/index.tsx file are moved to a final location
type Props = {
  dependencies: {
    service: ServiceContextType;
  };
};

export function CoreApp({ dependencies }: Props) {
  return (
    <ThemeProvider>
      <ServiceProvider {...dependencies.service}>
        <AppNavBar title="Michelangelo Studio" />
        <Router />
      </ServiceProvider>
    </ThemeProvider>
  );
}

export { useStudioQuery } from '#core/hooks/use-studio-query';
export { ServiceProvider } from '#core/providers/service-provider/service-provider';

export { getCellRenderer } from '#core/components/cell/get-cell-renderer';
export { BooleanCell } from '#core/components/cell/renderers/boolean/boolean-cell';
export { DateCell } from '#core/components/cell/renderers/date/date-cell';
