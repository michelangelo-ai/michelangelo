import { AppNavBar } from 'baseui/app-nav-bar';

import { Router } from '@/router/router';
import { ThemeProvider } from '@/themes/provider';

import '@/styles/main.css';

export function CoreApp() {
  return (
    <ThemeProvider>
      <AppNavBar title="Michelangelo Studio" />
      <Router />
    </ThemeProvider>
  );
}
