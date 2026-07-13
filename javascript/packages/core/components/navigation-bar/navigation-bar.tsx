import { AppNavBar } from 'baseui/app-nav-bar';
import { Button, KIND, SIZE } from 'baseui/button';

import { Link } from '#core/components/link/link';
import { useUserProvider } from '#core/providers/user-provider/use-user-provider';
import { StableUserMenuButton } from './stable-user-menu-button';

import type { NavItem } from 'baseui/app-nav-bar';
import type { Theme } from 'baseui/theme';
import type { LinkNavItem, NavigationLink } from './types';

type Props = {
  links?: NavigationLink[];
};

const USER_MENU_ITEMS: NavItem[] = [{ label: 'Sign out' }];

export function NavigationBar({ links }: Props) {
  const user = useUserProvider();

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
        // cast: NavItem.info is typed as `any` in BaseUI; mainItems always carry LinkNavItem shape
        const info = (item as LinkNavItem).info;
        if (!info?.href) return <>{item.label}</>;
        return (
          <Button
            href={info.href}
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
      username={user.name}
      usernameSubtitle={user.email}
      userImgUrl={user.avatarUrl}
      userItems={USER_MENU_ITEMS}
      overrides={{
        Root: { style: { position: 'relative' as const } },
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
        UserMenuButton: {
          component: StableUserMenuButton,
          props: {
            overrides: {
              BaseButton: {
                style: ({ $theme }: { $theme: Theme }) => ({
                  gap: $theme.sizing.scale200,
                }),
              },
            },
          },
        },
      }}
    />
  );
}
