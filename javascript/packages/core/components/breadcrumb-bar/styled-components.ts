import { Link } from 'react-router-dom-v5-compat';
import { styled } from 'baseui';

export const PlainLink = styled(Link, ({ $theme }) => ({
  textDecoration: 'none',
  color: $theme.colors.contentTertiary,
  ':visited': { color: $theme.colors.contentTertiary },
  ':hover': { textDecoration: 'underline' },
}));

export const BreadcrumbContainer = styled('div', ({ $theme }) => ({
  boxShadow: `inset 0px -1px 0px ${$theme.colors.borderOpaque}`,
  paddingTop: $theme.sizing.scale650,
  paddingBottom: $theme.sizing.scale650,
}));
