/**
 * Which casing rule applies to entity/phase names in a region of the tree,
 * per Uber's style guide:
 * - `nav` — wayfinding chrome that's scanned, not read as a sentence:
 *   breadcrumbs, left-nav, tabs, page H1s. Rendered in Title Case.
 * - `content` — prose read as a sentence: buttons, form headings, paragraphs.
 *   Rendered as written (sentence case).
 */
export type DisplayContextType = 'nav' | 'content';
