import { UserContext } from './user-context';

import type { UserContextType } from './types';

/**
 * Provides authenticated user data to components that render user-aware UI.
 */
export const UserProvider = ({
  children,
  ...userContext
}: { children: React.ReactNode } & UserContextType) => {
  return <UserContext.Provider value={userContext}>{children}</UserContext.Provider>;
};
