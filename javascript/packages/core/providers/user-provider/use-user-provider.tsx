import { useContext } from 'react';

import { UserContext } from './user-context';

/**
 * Accesses the current user information and authentication state.
 *
 * This hook must be used within a UserProvider component. It provides access
 * to the authenticated user's data.
 *
 * @returns User context containing the current user information. Returns default
 *   context values if used outside of a UserProvider.
 *
 * @example
 * ```typescript
 * function UserProfile() {
 *   const user = useUserProvider();
 *
 *   return (
 *     <div>
 *       <h1>Welcome, {user.name}</h1>
 *       <p>Email: {user.email}</p>
 *     </div>
 *   );
 * }
 * ```
 */
export const useUserProvider = () => {
  return useContext(UserContext);
};
