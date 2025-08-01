import { styled } from 'baseui';

export const FilterOptionItem = styled('li', ({ $theme }) => ({
  ...$theme.typography.ParagraphSmall,
  display: 'flex',
  justifyContent: 'space-between',
  alignItems: 'center',
  paddingTop: $theme.sizing.scale500,
  paddingBottom: $theme.sizing.scale500,
  paddingLeft: $theme.sizing.scale600,
  paddingRight: $theme.sizing.scale800,
  cursor: 'pointer',
  position: 'relative',

  ':hover': {
    backgroundColor: $theme.colors.menuFillHover,
  },

  ':before': getPseudoDividerLineStyles($theme.colors.borderOpaque),
}));

function getPseudoDividerLineStyles(color: string) {
  return {
    position: 'absolute',
    width: '85%',
    content: '""',
    borderTop: '1px solid',
    borderColor: color,
    top: 0,
    left: '7.5%',
  };
}
