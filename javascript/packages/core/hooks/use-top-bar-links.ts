import { DEFAULT_TOP_BAR_LINKS } from '#core/constants/top-bar-links';

import type { TopBarLink } from '#core/constants/types';

/**
 * Builds the `mainItems`/`onMainItemSelect` pair for AppNavBar's top-bar links.
 *
 * Adopters can pass their own `links` list to replace the Docs/Help defaults entirely — the
 * list isn't limited to a fixed Docs/Help pair, so adopters can add, rename, or drop entries
 * freely.
 */
export function useTopBarLinks(links: TopBarLink[] = DEFAULT_TOP_BAR_LINKS) {
  const urlByLabel = new Map(links.map((link) => [link.label, link.url]));

  const mainItems = links.map((link) => ({ label: link.label }));

  const handleLinkSelect = (item: { label: string }) => {
    const url = urlByLabel.get(item.label);
    if (url) {
      window.open(url, '_blank', 'noopener,noreferrer');
    }
  };

  return { mainItems, handleLinkSelect };
}
