import type { TopBarLink } from './types';

/** Shown in the top nav bar when no adopter-provided `links` are given. */
export const DEFAULT_TOP_BAR_LINKS: TopBarLink[] = [
  { label: 'Docs', url: 'https://michelangelo-ai.org/docs/' },
  { label: 'Help', url: 'https://michelangelo-ai.org/docs/#support--community' },
];
