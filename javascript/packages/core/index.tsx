import { AppNavBar } from 'baseui/app-nav-bar';

import { QueryProvider } from '#core/providers/query-provider/query-provider';
import { Router } from '#core/router/router';
import { ThemeProvider } from '#core/themes/provider';

import type { QueryContextType } from '#core/providers/query-provider/types';

import '#core/styles/main.css';

export function CoreApp(queryContext: QueryContextType) {
  return (
    <ThemeProvider>
      <QueryProvider {...queryContext}>
        <AppNavBar title="Michelangelo Studio" />
        <Router />
      </QueryProvider>
    </ThemeProvider>
  );
}
