import { withStyle } from 'baseui';
import { StyledLink as BaseStyledLink } from 'baseui/link';

import { Icon } from '#core/components/icon/icon';
import { IconKind } from '#core/components/icon/types';
import { EXTERNAL_LINK_ICON_SIZE } from './constants';

import type { IconProps } from 'baseui/icon';

export const StyledExternalLinkIcon = (props: IconProps) => {
  return (
    <Icon
      kind={IconKind.TERTIARY}
      style={{ flexShrink: 0, position: 'relative', top: '0.1em' }}
      {...props}
      name="arrowLaunch"
      size={EXTERNAL_LINK_ICON_SIZE}
    />
  );
};

export const StyledLink = withStyle<typeof BaseStyledLink, { $external?: boolean }>(
  BaseStyledLink,
  ({ $external, $theme }) => ({
    ':hover': { textDecoration: 'underline' },
    ':visited': { color: $theme.colors.contentSecondary },
    alignItems: 'center',
    display: 'inline-flex',
    fontWeight: 'unset',
    gap: $theme.sizing.scale100,
    maxWidth: $external ? `calc(100% - ${EXTERNAL_LINK_ICON_SIZE})` : '100%',
    textDecoration: 'none',
    // For some reason, the ExternalLinkIcon's width isn't factored into the overall anchor
    // width. This results in overflow that is especially pronounced in grid layouts, e.g.,
    // form rows. whiteSpace: nowrap added to prevent the < 100% maxWidth from wrapping text
    whiteSpace: 'nowrap',
    width: 'fit-content',
  })
);
