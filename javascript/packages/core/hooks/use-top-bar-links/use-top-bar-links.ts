import type { TopBarLinks } from './types';

const DEFAULT_DOCS_URL = 'https://michelangelo-ai.org/docs/';
const DEFAULT_HELP_URL = 'https://michelangelo-ai.org/docs/#support--community';

const LABEL = {
  DOCS: 'Docs',
  HELP: 'Help',
} as const;

/**
 * Builds the `mainItems`/`onMainItemSelect` pair for AppNavBar's Docs and Help links.
 *
 * Each adopter can point these at their own documentation and support pages by passing
 * `links` — unset fields fall back to the Michelangelo OSS docs site, so adopters that don't
 * maintain their own docs get a working default.
 */
export function useTopBarLinks(links: TopBarLinks = {}) {
  const hrefByLabel: Record<string, string> = {
    [LABEL.DOCS]: links.docsUrl ?? DEFAULT_DOCS_URL,
    [LABEL.HELP]: links.helpUrl ?? DEFAULT_HELP_URL,
  };

  const mainItems = [{ label: LABEL.DOCS }, { label: LABEL.HELP }];

  const handleMainItemSelect = (item: { label: string }) => {
    const href = hrefByLabel[item.label];
    if (href) {
      window.open(href, '_blank', 'noopener,noreferrer');
    }
  };

  return { mainItems, handleMainItemSelect };
}
