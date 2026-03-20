import { Outlet } from 'react-router-dom-v5-compat';

import { BreadcrumbBar } from '#core/components/breadcrumb-bar/breadcrumb-bar';

import type { CategoryConfig } from '#core/types/common/studio-types';

interface Props {
  categories: CategoryConfig[];
}

/**
 * Layout route component that renders Studio navigation bar above all project-level pages.
 *
 * Renders as a pathless Route element so child routes inherit its matched params,
 * allowing useStudioParams (and useParams) to resolve correctly inside BreadcrumbBar.
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
export function StudioBar({ categories }: Props) {
  return (
    <>
      <BreadcrumbBar categories={categories} />
      <Outlet />
    </>
  );
}
