import { AppNavBar } from 'baseui/app-nav-bar';
import { Button, KIND, SIZE } from 'baseui/button';

import { Link } from '#core/components/link/link';

import type { Theme } from 'baseui/theme';
import type { LinkNavItem, NavigationLink } from './types';

type Props = {
  links?: NavigationLink[];
};

export function NavigationBar({ links }: Props) {
  const mainItems: LinkNavItem[] =
    links?.map((link) => ({
      label: link.label,
      info: { href: link.href },
    })) ?? [];

  return (
    <AppNavBar
      title={
        <Link href="/" overrides={{ Link: { style: { ':hover': { textDecoration: 'unset' } } } }}>
          Michelangelo Studio
        </Link>
      }
      mainItems={mainItems}
      mapItemToNode={(item) => {
        // cast: see LinkNavItem's doc comment in types.ts
        const { href } = (item as LinkNavItem).info;
        return (
          <Button
            href={href}
            target="_blank"
            rel="noopener noreferrer"
            kind={KIND.tertiary}
            size={SIZE.compact}
            overrides={{
              BaseButton: {
                style: {
                  display: 'flex',
                  alignItems: 'flex-start',
                  whiteSpace: 'nowrap',
                },
              },
            }}
          >
            {item.label}
          </Button>
        );
      }}
      overrides={{
        AppName: { style: { whiteSpace: 'nowrap' } },
        PrimaryMenuContainer: {
          style: ({ $theme }: { $theme: Theme }) => ({
            marginLeft: $theme.sizing.scale1000,
          }),
        },
        DesktopMenu: {
          style: {
            height: '64px',
            boxSizing: 'border-box',
            paddingBlockStart: '20px',
          },
        },
      }}
    />
  );
}
