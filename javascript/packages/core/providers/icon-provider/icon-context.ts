import { createContext } from 'react';

import type { IconProviderContext } from './types';

export const IconContext = createContext<IconProviderContext>({
  icons: {},
});
