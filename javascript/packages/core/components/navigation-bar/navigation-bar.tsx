import { AppNavBar } from 'baseui/app-nav-bar';
import { Button, KIND, SIZE } from 'baseui/button';

import { Link } from '#core/components/link/link';
import { useUserProvider } from '#core/providers/user-provider/use-user-provider';
import { StableUserMenuButton } from './stable-user-menu-button';

import type { NavItem } from 'baseui/app-nav-bar';
import type { Theme } from 'baseui/theme';
import type { LinkNavItem, NavigationLink, UserMenuItem } from './types';

type Props = {
  links?: NavigationLink[];
  userMenuItems?: UserMenuItem[];
};

export function NavigationBar({ links, userMenuItems }: Props) {
  const user = useUserProvider();

  const mainItems: LinkNavItem[] =
    links?.map((link) => ({
      label: link.label,
      info: { href: link.href },
    })) ?? [];

  const userItems: NavItem[] =
    userMenuItems?.map((item) => ({
      label: item.label,
      info: { onClick: item.onClick, icon: item.icon },
    })) ?? [];

  const handleUserMenuItemSelect = (item: NavItem) => {
    // cast: NavItem.info is typed as `any` in BaseUI
    const info = item.info as { onClick?: () => void } | undefined;
    info?.onClick?.();
  };

  return (
    <AppNavBar
      title={
        <Link href="/" overrides={{ Link: { style: { ':hover': { textDecoration: 'unset' } } } }}>
          Michelangelo Studio
        </Link>
      }
      mainItems={mainItems}
      mapItemToNode={(item) => {
        // cast: see LinkNavItem in types.ts
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
      username={user.name}
      usernameSubtitle={user.email}
      userImgUrl={user.avatarUrl}
      userItems={userItems}
      onUserItemSelect={handleUserMenuItemSelect}
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
