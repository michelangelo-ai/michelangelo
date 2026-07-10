import { AppNavBar } from 'baseui/app-nav-bar';
import { Button, KIND, SIZE } from 'baseui/button';

import { Link } from '#core/components/link/link';

import type { NavItem } from 'baseui/app-nav-bar';
import type { Theme } from 'baseui/theme';
import type { NavigationLink } from './types';

type Props = {
  links?: NavigationLink[];
};

export function NavigationBar({ links }: Props) {
  const mainItems: NavItem[] =
    links?.map((link) => ({
      label: link.label,
      info: { href: link.href },
    })) ?? [];

  const handleNavigationLinkSelect = (item: NavItem) => {
    // cast: NavItem.info is typed as `any` in BaseUI
    const info = item.info as { href: string } | undefined;
    if (info?.href) {
      window.open(info.href, '_blank', 'noopener,noreferrer');
    }
  };

  return (
    <AppNavBar
      title={
        <Link href="/" overrides={{ Link: { style: { ':hover': { textDecoration: 'unset' } } } }}>
          Michelangelo Studio
        </Link>
      }
      mainItems={mainItems}
      onMainItemSelect={handleNavigationLinkSelect}
      mapItemToNode={(item) => (
        <Button
          kind={KIND.tertiary}
          size={SIZE.compact}
          overrides={{
            BaseButton: {
              style: ({ $theme }: { $theme: Theme }) => ({
                display: 'flex',
                alignItems: 'flex-start',
                whiteSpace: 'nowrap',
                backgroundColor: 'transparent',
                ':hover': {
                  backgroundColor: $theme.colors.backgroundSecondary,
                },
              }),
            },
          }}
        >
          {item.label}
        </Button>
      )}
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
