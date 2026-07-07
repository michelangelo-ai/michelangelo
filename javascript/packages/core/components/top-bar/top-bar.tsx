import { AppNavBar } from 'baseui/app-nav-bar';

import { AppTitle } from '#core/components/app-title/app-title';
import { useTopBarLinks } from '#core/hooks/use-top-bar-links';

import type { TopBarProps } from './types';

/**
 * Michelangelo's top nav bar: app title plus a configurable list of links (Docs/Help by default).
 *
 * Wired together as one component (rather than composed inline in `CoreApp`) so the title
 * styling and link behavior can be tested as what a user actually sees and clicks, instead of
 * testing the hook and title in isolation from how they're wired into the nav bar.
 */
export function TopBar({ links }: TopBarProps) {
  const { mainItems, handleLinkSelect } = useTopBarLinks(links);

  return (
    <AppNavBar
      title={<AppTitle>Michelangelo Studio</AppTitle>}
      mainItems={mainItems}
      onMainItemSelect={handleLinkSelect}
    />
  );
}
