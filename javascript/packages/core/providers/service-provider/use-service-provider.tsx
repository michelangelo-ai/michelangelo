import { useContext } from 'react';

import { ServiceContext } from './service-context';

export const useServiceProvider = () => {
  return useContext(ServiceContext);
};
