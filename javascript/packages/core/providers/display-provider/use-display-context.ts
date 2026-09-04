import { useContext } from 'react';

import { DisplayContext } from './display-context';

/**
 * Reads the nearest `DisplayProvider` ancestor's type, or `undefined` if
 * nothing in the tree has declared one.
 */
export function useDisplayContext() {
  return useContext(DisplayContext);
}
