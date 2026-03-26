/**
 * A top-level navigation link rendered in the menu drawer above the project/phase
 * hierarchy. Intended for destinations that live outside the project context.
 *
 * `path` is resolved relative to the root router — the same base path that `"/"` in
 * `BreadcrumbBar` resolves to.
 */
export interface NavLink {
  /** Display label shown in the drawer */
  label: string;
  /** Route path relative to the root router (e.g. "/settings") */
  path: string;
}
