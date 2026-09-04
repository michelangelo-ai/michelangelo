import { useDisplayContext } from '#core/providers/display-provider/use-display-context';

import type { DisplayContextType } from '#core/providers/display-provider/types';

/**
 * Returns `name` cased for the surrounding `DisplayProvider`. Pass `casing`
 * to override the ambient context for a one-off exception; with no ambient
 * context and no override, `name` is returned unchanged.
 *
 * @example
 * ```tsx
 * // Inside <DisplayProvider type="nav">:
 * useEntityName('trained models'); // 'Trained Models'
 *
 * // Inside <DisplayProvider type="content">:
 * useEntityName('trained models'); // 'trained models'
 *
 * // Explicit override, regardless of ambient context:
 * useEntityName('trained models', 'content'); // 'trained models'
 * ```
 */
export function useEntityName(name: string, casing?: DisplayContextType): string {
  const context = useDisplayContext();
  return formatEntityName(name, casing ?? context);
}

/**
 * Applies display casing for a given context value directly, without reading
 * `DisplayProvider` from the component tree. Use this over `useEntityName`
 * when casing several names against a context read once — e.g. mapping a
 * list — since a hook can't be called once per loop iteration.
 */
export function formatEntityName(name: string, casing: DisplayContextType | undefined): string {
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
