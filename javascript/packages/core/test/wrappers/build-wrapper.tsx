import type { RenderOptions } from '@testing-library/react';
import type { ReactNode } from 'react';

import { Wrappers } from './types';

/**
 * Creates a testing wrapper that composes multiple React Testing Library wrappers.
 * This is useful for testing components that require multiple context providers
 * (e.g., ThemeProvider, Router, etc.).
 *
 * @param wrappers - Array of wrapper components to compose
 * @returns A wrapper configuration compatible with React Testing Library's render options
 *
 * @example
 * ```tsx
 * const wrapper = buildWrapper([getRouterWrapper()]);
 * render(<MyComponent />, wrapper);
 * ```
 */
export function buildWrapper(wrappers: Wrappers): Pick<RenderOptions, 'wrapper'> {
  return {
    wrapper: ({ children }) => {
      return wrappers.reduce<ReactNode>(
        (result, Wrapper, index) => <Wrapper key={index}>{result}</Wrapper>,
        children
      );
    },
  };
}
