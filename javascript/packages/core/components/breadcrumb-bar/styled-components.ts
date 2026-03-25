import { Link } from 'react-router-dom-v5-compat';
import { styled } from 'baseui';

export const PlainLink = styled(Link, ({ $theme }) => ({
  textDecoration: 'none',
  color: $theme.colors.contentTertiary,
  ':visited': { color: $theme.colors.contentTertiary },
  ':hover': { textDecoration: 'underline' },
}));

export const TopLevelNavLink = styled(Link, ({ $theme }) => ({
  ...$theme.typography.LabelMedium,
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'space-between',
  height: $theme.sizing.scale1200,
  paddingLeft: $theme.sizing.scale600,
  paddingRight: $theme.sizing.scale700,
  textDecoration: 'none',
  // Override the default visited link color
  ':visited': { color: $theme.colors.contentPrimary },
  ':hover': { backgroundColor: $theme.colors.menuFillHover },
}));

export const BreadcrumbContainer = styled('div', ({ $theme }) => ({
  boxShadow: `inset 0px -1px 0px ${$theme.colors.borderOpaque}`,
  paddingTop: $theme.sizing.scale650,
  paddingBottom: $theme.sizing.scale650,
}));

export const PhaseHeader = styled<'li', { $disabled?: boolean }>('li', (props) => {
  return {
    ...props.$theme.typography.LabelMedium,
    color: props.$disabled
      ? props.$theme.colors.contentTertiary
      : props.$theme.colors.contentPrimary,
    paddingTop: props.$theme.sizing.scale500,
    paddingBottom: props.$theme.sizing.scale300,
    paddingLeft: props.$theme.sizing.scale800,
    paddingRight: props.$theme.sizing.scale800,
    display: 'flex',
    alignItems: 'center',
    whiteSpace: 'nowrap',
    gap: props.$theme.sizing.scale600,
  };
});

export const EntityItem = styled<'li', { $disabled?: boolean }>('li', ({ $theme, $disabled }) => ({
  cursor: $disabled ? 'not-allowed' : 'pointer',
  paddingTop: $theme.sizing.scale200,
  paddingBottom: $theme.sizing.scale200,
  paddingLeft: $theme.sizing.scale1600,
  paddingRight: $theme.sizing.scale800,
  ':hover': {
    backgroundColor: $disabled ? undefined : $theme.colors.menuFillHover,
  },
  transitionProperty: 'background-color',
  transitionDuration: $theme.animation.timing200,
  transitionTimingFunction: $theme.animation.easeOutCurve,
}));
