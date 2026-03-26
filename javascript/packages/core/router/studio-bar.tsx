import { Outlet } from 'react-router-dom-v5-compat';

import { BreadcrumbBar } from '#core/components/breadcrumb-bar/breadcrumb-bar';

import type { NavLink } from '#core/components/breadcrumb-bar/types';
import type { CategoryConfig } from '#core/types/common/studio-types';

interface Props {
  categories: CategoryConfig[];
  topLevelLinks?: NavLink[];
}

/**
 * Layout route component that renders Studio navigation bar above all project-level pages.
 *
 * Renders as a pathless Route element so child routes inherit its matched params,
 * allowing useStudioParams (and useParams) to resolve correctly inside BreadcrumbBar.
 *
 * Top-level links (e.g. to pages outside the project context) can be provided via
 * `topLevelLinks` and appear at the top of the menu drawer.
 *
 * @example
 * ```tsx
 * <Routes>
 *   <Route element={<StudioBar categories={CATEGORIES} />}>
 *     <Route path=":projectId" element={<ProjectDetail />} />
 *   </Route>
 * </Routes>
 * ```
 */
export function StudioBar({ categories, topLevelLinks }: Props) {
  return (
    <>
      <BreadcrumbBar categories={categories} topLevelLinks={topLevelLinks} />
      <Outlet />
    </>
  );
}
