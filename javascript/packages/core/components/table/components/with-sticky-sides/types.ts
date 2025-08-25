export interface WithStickySidesProps {
  enableStickySides: boolean;
  /** Whether the table has row selection checkboxes (affects column positioning) */
  enableRowSelection: boolean;
  /** Zero-based index of the rightmost column (used for right-side sticky positioning) */
  lastColumnIndex: number;
  /** Current horizontal scroll ratio (-1 = no scroll, 0 = left edge, 1 = right edge) */
  scrollRatio: number;
  /** ARIA role for the wrapped component */
  role: string;
  children: React.ReactNode;
}

/**
 * Configuration for a single sticky column's positioning and shadow behavior.
 */
export interface StickyColumnConfig {
  stickySide: 'left' | 'right';
  /** Offset in pixels from the sticky side (accounts for other sticky columns) */
  position: number;
  shadowSide: ShadowSide;
}

/**
 * Map of column indices to their sticky configurations.
 */
export type StickyColumnConfigs = Record<number, StickyColumnConfig>;

/**
 * Direction for shadow effects on sticky columns.
 *
 * - 'left': Shadow when content scrolled off to the left
 * - 'right': Shadow when content scrolled off to the right
 * - 'none': No shadow effects (selection column)
 */
export type ShadowSide = 'left' | 'right' | 'none';
