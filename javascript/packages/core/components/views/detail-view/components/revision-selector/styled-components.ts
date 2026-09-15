import { styled } from 'baseui';

export const RevisionList = styled('div', ({ $theme }) => ({
  backgroundColor: $theme.colors.backgroundPrimary,
  borderRadius: $theme.borders.radius300,
  display: 'flex',
  flexDirection: 'column',
  maxHeight: '60vh',
  minWidth: '560px',
  overflowY: 'auto',
  paddingTop: $theme.sizing.scale300,
  paddingBottom: $theme.sizing.scale300,
}));

export const RevisionListHeader = styled('div', ({ $theme }) => ({
  ...$theme.typography.LabelMedium,
  borderBottom: `1px solid ${$theme.colors.borderOpaque}`,
  paddingTop: $theme.sizing.scale500,
  paddingBottom: $theme.sizing.scale500,
  paddingLeft: $theme.sizing.scale600,
  paddingRight: $theme.sizing.scale600,
}));

export const RevisionRow = styled<'button', { $isSelected: boolean }>(
  'button',
  ({ $theme, $isSelected }) => ({
    ...$theme.typography.ParagraphMedium,
    fontWeight: $isSelected ? 'bold' : 'normal',
    backgroundColor: $isSelected ? $theme.colors.backgroundSecondary : 'transparent',
    borderTopWidth: 0,
    borderRightWidth: 0,
    borderBottomWidth: 0,
    borderLeftWidth: 0,
    color: $theme.colors.contentPrimary,
    cursor: 'pointer',
    textAlign: 'left',
    width: '100%',
    paddingTop: $theme.sizing.scale500,
    paddingBottom: $theme.sizing.scale500,
    paddingLeft: $theme.sizing.scale600,
    paddingRight: $theme.sizing.scale600,
    ':hover': { backgroundColor: $theme.colors.backgroundTertiary },
  })
);

export const RevisionColumns = styled('span', ({ $theme }) => ({
  display: 'grid',
  gridTemplateColumns: '2fr 2fr 3fr',
  columnGap: $theme.sizing.scale800,
  alignItems: 'center',
}));
