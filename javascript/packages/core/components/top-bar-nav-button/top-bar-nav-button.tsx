import { Button, KIND, SIZE } from 'baseui/button';

import type { NavItem } from 'baseui/app-nav-bar';

/** Renders an AppNavBar main item (e.g. Docs, Help) as a plain text button. */
export function TopBarNavButton(item: NavItem) {
  return (
    <Button kind={KIND.tertiary} size={SIZE.compact}>
      {item.label}
    </Button>
  );
}
