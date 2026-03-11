import { styled } from 'baseui';

// Spacer that occupies the same height as the fixed footer, preventing page
// content from being hidden behind it when scrolled to the bottom.
export const StickyFooterSpacer = styled<'div', { $height: number }>('div', ({ $height }) => ({
  height: `${$height}px`,
  flexShrink: 0,
}));

export const StickyFooterContainer = styled('footer', ({ $theme }) => ({
  display: 'flex',
  justifyContent: 'space-between',
  alignItems: 'center',
  width: '100%',
  position: 'fixed',
  boxSizing: 'border-box',
  left: 0,
  bottom: 0,
  boxShadow: $theme.lighting.shallowAbove,
  backgroundColor: $theme.colors.backgroundPrimary,
  paddingTop: $theme.sizing.scale600,
  paddingBottom: $theme.sizing.scale600,
  paddingLeft: $theme.sizing.scale900,
  paddingRight: $theme.sizing.scale900,
  gap: $theme.sizing.scale400,
}));

export const StickyFooterSlot = styled('div', ({ $theme }) => ({
  display: 'flex',
  alignItems: 'center',
  gap: $theme.sizing.scale200,
}));
