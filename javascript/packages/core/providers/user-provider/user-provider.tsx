import { UserContext } from './user-context';

import type { UserContextType } from './types';

export const UserProvider = ({
  children,
  ...userContext
}: { children: React.ReactNode } & UserContextType) => {
  return <UserContext.Provider value={userContext}>{children}</UserContext.Provider>;
};
