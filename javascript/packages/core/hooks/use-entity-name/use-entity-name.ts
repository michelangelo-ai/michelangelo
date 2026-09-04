import { useDisplayContext } from '#core/providers/display-context/use-display-context';

import type { DisplayContextType } from '#core/providers/display-context/types';

/**
 * Capitalizes the first letter of each space-separated word. Splitting on
 * spaces only (not hyphens) preserves hyphenated names like "one-off" as a
 * single word, and capitalizing only the first character is a no-op on
 * already-uppercase letters, so acronyms stored in the canonical name (e.g.
 * "AI pipelines") survive untouched.
 */
function toNavCase(name: string): string {
  return name
    .split(' ')
    .map((word) => (word ? word[0].toUpperCase() + word.slice(1) : word))
    .join(' ');
}

/**
 * Applies the display casing for a given context value directly, without
 * reading `DisplayContext` from the component tree. `useEntityName` is a thin
 * wrapper around this for the common case; use this directly when casing
 * several names against a context read once — e.g. mapping a list — since a
 * hook can't be called once per loop iteration.
 */
export function formatEntityName(name: string, casing: DisplayContextType | undefined): string {
  return casing === 'nav' ? toNavCase(name) : name;
}

/**
 * @description
 * Returns `name` cased for the surrounding `DisplayContext` — Title Case in a
 * `"nav"` region, passed through as-is in a `"content"` region (the canonical
 * form is already lowercase/sentence case). Pass `casing` to override the
 * ambient context for a one-off exception; with no ambient context and no
 * override, `name` is returned unchanged.
 *
 * This replaces the old `startCaseEntityName`/`toSentenceCaseName` helpers —
 * casing is now a property of where a name is rendered, not a choice made by
 * whichever helper a call site happens to import.
 *
 * @example
 * ```tsx
 * // Inside <DisplayContext type="nav">:
 * useEntityName('trained models'); // 'Trained Models'
 *
 * // Inside <DisplayContext type="content">:
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
