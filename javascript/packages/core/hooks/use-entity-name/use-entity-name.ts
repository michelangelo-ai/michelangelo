/**
 * Applies nav casing (Title Case) to an entity name, or returns it unchanged.
 * Nav regions are wayfinding chrome that's scanned, not read as a sentence —
 * breadcrumbs, left-nav, tabs, page H1s — per Uber's style guide. Everywhere
 * else, the lowercase canonical config value IS the correct display form, so
 * there's nothing to pass for `casing`.
 *
 * @example
 * ```tsx
 * formatEntityName('trained models', 'nav'); // 'Trained Models'
 * formatEntityName('trained models'); // 'trained models'
 * ```
 */
export function formatEntityName(name: string, casing?: 'nav'): string {
  return casing === 'nav' ? toNavCase(name) : name;
}

/**
 * Capitalizes the first letter of each space-separated word. Splitting on
 * spaces (not hyphens) keeps hyphenated names like "one-off" intact, and
 * already-uppercase letters are untouched, so acronyms survive.
 */
function toNavCase(name: string): string {
  return name
    .split(' ')
    .map((word) => (word ? word[0].toUpperCase() + word.slice(1) : word))
    .join(' ');
}
