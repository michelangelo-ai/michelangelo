import { useContext } from 'react';

import { IconContext } from './icon-context';

/**
 * Accesses the icon registry to render custom icons throughout the application.
 *
 * This hook must be used within an IconProvider component. It provides access
 * to the icon registry which maps icon names to React components.
 *
 * @returns Icon context containing the icon registry mapping. Returns default empty
 *   registry if used outside of an IconProvider.
 *
 * @example
 * ```typescript
 * function MyComponent() {
 *   const { icons } = useIconProvider();
 *
 *   // Get a specific icon component
 *   const PlayIcon = icons.play;
 *
 *   return <PlayIcon />;
 * }
 * ```
 */
export const useIconProvider = () => {
  return useContext(IconContext);
};
