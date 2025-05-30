import { createContext } from 'react';

import { UserTimeZone } from './types';

import type { UserContextType } from './types';

export const UserContext = createContext<UserContextType>({
  timeZone: UserTimeZone.Local,
});
