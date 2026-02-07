import { Link as RouterLink } from 'react-router-dom-v5-compat';
import { getOverrides } from 'baseui';

import { isAbsoluteURL } from '#core/utils/string-utils';
import { StyledLink } from './styled-components';
import { StyledExternalLinkIcon } from './styled-components';

import type { LinkProps } from './types';

/**
 * Smart link component that automatically handles internal navigation and external links.
 *
 * This component determines whether a link is internal (same application) or external
 * (different domain) and renders the appropriate link type:
 * - Internal links use React Router for client-side navigation
 * - External links open in a new tab with security attributes and an external link icon
 *
 * Features:
 * - Automatic internal vs external detection via URL validation
 * - External links open in new tab with rel="noopener noreferrer"
 * - Visual external link indicator icon
 * - Customizable through BaseUI overrides
 * - Theme integration
 *
 * @param props.href - Destination URL. Can be relative (internal) or absolute (external)
 * @param props.children - Link text or content to display
 * @param props.title - Optional title attribute for accessibility
 * @param props.overrides - BaseUI overrides for Link and ExternalLinkIcon components
 *
 * @example
 * ```tsx
 * // Internal navigation (uses React Router)
 * <Link href="/pipelines/my-pipeline">View Pipeline</Link>
 *
 * // External link (opens in new tab with icon)
 * <Link href="https://docs.example.com">Documentation</Link>
 *
 * // With custom styling
 * <Link
 *   href="/settings"
 *   overrides={{
 *     Link: {
 *       style: { fontWeight: 'bold' }
 *     }
 *   }}
 * >
 *   Settings
 * </Link>
 * ```
 */
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
