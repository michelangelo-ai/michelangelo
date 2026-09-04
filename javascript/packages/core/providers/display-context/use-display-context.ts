import { useContext } from 'react';

import { DisplayContextValue } from './display-context-context';

/**
 * Reads the nearest `DisplayContext` ancestor's type, or `undefined` if
 * nothing in the tree has declared one.
 */
export function useDisplayContext() {
  return useContext(DisplayContextValue);
}
