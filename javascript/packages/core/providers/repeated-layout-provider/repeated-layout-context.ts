import { createContext } from 'react';

import type { RepeatedLayoutState } from './types';

export const RepeatedLayoutContext = createContext<RepeatedLayoutState | undefined>(undefined);
