import { Link as RouterLink } from 'react-router-dom-v5-compat';
import { getOverrides } from 'baseui';

import { isAbsoluteURL } from '#core/utils/string-utils';
import { StyledLink } from './styled-components';
import { StyledExternalLinkIcon } from './styled-components';

import type { LinkProps } from './types';

export function Link(props: LinkProps) {
  const { children, href, overrides = {}, title } = props;

  const [Link, linkProps] = getOverrides(overrides.Link, StyledLink);

  const [ExternalLinkIcon, externalLinkIconProps] = getOverrides(
    overrides?.ExternalLinkIcon,
    StyledExternalLinkIcon
  );

  return isAbsoluteURL(href) ? (
    <Link
      $external
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      title={title}
      {...linkProps}
    >
      {children}
      <ExternalLinkIcon title="External link" {...externalLinkIconProps} />
    </Link>
  ) : (
    <Link $as={RouterLink} to={href} title={title} {...linkProps}>
      {children}
    </Link>
  );
}
