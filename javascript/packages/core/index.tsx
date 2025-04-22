import { Router } from '#core/router/router';
import { ThemeProvider } from '#core/themes/provider';
import { AppNavBar } from 'baseui/app-nav-bar';

import '#core/styles/main.css';

export function CoreApp() {
  return (
    <ThemeProvider>
      <AppNavBar title="Michelangelo Studio" />
      <Router />
    </ThemeProvider>
  );
}
