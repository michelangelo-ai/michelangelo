import { useContext } from 'react';

import { DisplayContext } from './display-context';

/** Reads the nearest `DisplayProvider` ancestor's type, or `undefined` if none. */
export function useDisplayContext() {
  return useContext(DisplayContext);
}
