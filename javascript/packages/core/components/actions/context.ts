import { createContext } from 'react';

import type { ActionContextValue, ActionMenuContextValue } from './types';

export const ActionMenuContext = createContext<ActionMenuContextValue>({
  closeMenu: () => undefined,
  openAction: () => undefined,
});

export const ActionContext = createContext<ActionContextValue>({});
