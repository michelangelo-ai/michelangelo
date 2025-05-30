import { useContext } from 'react';

import { UserContext } from './user-context';

export const useUserProvider = () => {
  return useContext(UserContext);
};
