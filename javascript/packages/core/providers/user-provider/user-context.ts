import { createContext } from 'react';

import { TimeZone } from '#core/types/time-types';

import type { UserContextType } from './types';

export const UserContext = createContext<UserContextType>({
  timeZone: TimeZone.Local,
});
