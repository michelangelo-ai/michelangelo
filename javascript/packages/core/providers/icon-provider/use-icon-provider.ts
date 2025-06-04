import { useContext } from 'react';

import { IconContext } from './icon-context';

export const useIconProvider = () => {
  return useContext(IconContext);
};
