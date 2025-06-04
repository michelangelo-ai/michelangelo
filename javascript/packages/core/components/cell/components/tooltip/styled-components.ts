import { styled } from 'baseui';

export const ClickableContainer = styled<
  'div',
  {
    onClick?: unknown;
  }
>('div', ({ $theme, onClick }) => ({
  cursor: onClick ? 'pointer' : 'inherit',
  fontSize: $theme.sizing.scale550,
  alignItems: 'center',
  display: 'flex',
  gap: '16px',
  paddingTop: '8px',
  paddingBottom: '8px',
  lineHeight: '18px',
}));
