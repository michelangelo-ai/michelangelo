import { AppNavBar } from 'baseui/app-nav-bar';

import { ServiceProvider } from '#core/providers/service-provider/service-provider';
import { Router } from '#core/router/router';
import { ThemeProvider } from '#core/themes/provider';

import type { ServiceContextType } from '#core/providers/service-provider/types';

import '#core/styles/main.css';

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
