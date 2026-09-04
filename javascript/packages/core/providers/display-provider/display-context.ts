import { createContext } from 'react';

import type { DisplayContextType } from './types';

/** `undefined` means no ancestor `DisplayProvider` — treated as no casing transform. */
export const DisplayContext = createContext<DisplayContextType | undefined>(undefined);
