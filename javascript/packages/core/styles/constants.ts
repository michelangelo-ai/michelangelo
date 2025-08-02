/**
 * Standard text overflow behavior for single-line text truncation.
 * Displays ellipsis (...) when text exceeds container width.
 */
export const ELLIPSIS_STYLES = {
  overflow: 'hidden',
  textOverflow: 'ellipsis',
  whiteSpace: 'nowrap',
} as const;
