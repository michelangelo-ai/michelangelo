import { InterpolationContextExtensions } from '#core/interpolation/types';
import { InterpolationContext } from './interpolation-context';

import type { ReactNode } from 'react';

/**
 * Provides shared data context for interpolations throughout the component tree.
 *
 * @example
 * ```tsx
 * // Provide user and project data to all child components
 * const currentUser = { name: 'John' }
 * const activeProject = { title: 'My Project' }
 * <InterpolationProvider user={currentUser} project={activeProject}>
 *   <Dashboard />
 * </InterpolationProvider>
 *
 * // Inside Dashboard or any child component:
 * const greeting = interpolate('Welcome ${user.name} to ${project.title}');
 * // Resolves to: "Welcome John to My Project"
 * ```
 */
export function InterpolationProvider({
  children,
  value,
}: {
  children: ReactNode;
  value?: InterpolationContextExtensions;
}) {
  return <InterpolationContext.Provider value={value}>{children}</InterpolationContext.Provider>;
}
