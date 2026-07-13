import type { NavItem } from 'baseui/app-nav-bar';

export type NavigationLink = {
  label: string;
  href: string;
};

/** BaseUI types `mainItems`/`mapItemToNode` against the generic `NavItem` (`info: any`); these are always the entries built from `NavigationLink`s. */
export type LinkNavItem = Omit<NavItem, 'info'> & { info: { href: string } };
