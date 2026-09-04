import { createContext } from 'react';

import type { DisplayContextType } from './types';

/**
 * `undefined` means no region has declared a display context — consumers
 * (e.g. `useEntityName`) treat that as "no casing transform".
 */
export const DisplayContext = createContext<DisplayContextType | undefined>(undefined);
