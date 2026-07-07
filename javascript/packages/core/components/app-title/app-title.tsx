import { Link } from 'react-router-dom-v5-compat';
import { useStyletron } from 'baseui';

import type { ReactNode } from 'react';

type AppTitleProps = {
  children: ReactNode;
};

/**
 * Clickable app name for the top nav bar, linking to `/`.
 *
 * Deliberately avoids the shared `Link` component: `AppNavBar` wraps its `title` prop in a
 * container that already applies heading-level typography and color, and the shared `Link`
 * sets its own explicit link color/font directly on the element, which overrides that heading
 * style. This component only adds navigation, inheriting whatever styling the parent provides.
 */
export function AppTitle({ children }: AppTitleProps) {
  const [css] = useStyletron();

  return (
    <Link to="/" className={css({ color: 'inherit', textDecoration: 'none' })}>
      {children}
    </Link>
  );
}
