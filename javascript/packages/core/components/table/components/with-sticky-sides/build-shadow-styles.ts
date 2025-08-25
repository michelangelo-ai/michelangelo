import type { StyleObject } from 'styletron-standard';
import type { ShadowSide } from './types';

const SHADOW_CONFIGS = {
  left: {
    right: 'auto',
    left: '-7px',
    getOpacity: (scrollRatio: number) => (scrollRatio === -1 ? 0 : 1 - scrollRatio),
  },
  right: {
    right: '-7px',
    left: 'auto',
    getOpacity: (scrollRatio: number) => (scrollRatio === -1 ? 0 : scrollRatio),
  },
} as const;

/**
 * Builds CSS styles for shadow effects on sticky table columns.
 *
 * Creates a visual shadow indicator that appears when content is scrolled
 * off-screen, helping users understand there's more content in that direction.
 *
 * Shadow behavior:
 * - Left shadow: Appears when scrolled right, fades as scroll approaches left edge
 * - Right shadow: Appears when scrolled left, fades as scroll approaches right edge
 * - No shadow: Returns empty styles (used for selection column)
 *
 * The shadow is implemented as a CSS ::before pseudo-element with:
 * - 7px width positioned outside the column
 * - Linear gradient from shadow color to transparent
 * - Opacity controlled by scroll position (-1 = no scroll, 0-1 = scroll ratio)
 *
 * @param shadowSide - Which side should show the shadow ('left', 'right', 'none')
 * @param scrollRatio - Current scroll position (-1 = no scroll, 0 = start, 1 = end)
 * @returns Style object with ::before pseudo-element for shadow effect
 */
export function buildShadowStyles(
  shadowSide: ShadowSide,
  scrollRatio: number
): { '::before'?: StyleObject } {
  if (shadowSide === 'none') return {};

  const { getOpacity, ...rest } = SHADOW_CONFIGS[shadowSide];

  return {
    '::before': {
      ...rest,
      content: '" "',
      position: 'absolute',
      top: 0,
      bottom: 0,
      width: '7px',
      opacity: Math.ceil(getOpacity(scrollRatio)),
      transition: 'opacity 0.2s ease',
      background: `linear-gradient(to ${shadowSide}, rgba(0,0,0, 0.15), rgba(0, 0, 0, 0) 4px)`,
    },
  };
}
