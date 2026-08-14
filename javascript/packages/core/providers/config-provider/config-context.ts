import { createContext } from 'react';

import type { StudioConfigContextType } from './types';

export const ConfigContext = createContext<StudioConfigContextType | null>(null);
