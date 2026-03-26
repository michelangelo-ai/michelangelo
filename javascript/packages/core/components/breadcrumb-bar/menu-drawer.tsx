import { useState } from 'react';
import { useStyletron } from 'baseui';
import { Button, KIND, SHAPE, SIZE } from 'baseui/button';
import { ANCHOR, Drawer } from 'baseui/drawer';

import { Icon } from '#core/components/icon/icon';
import { Tag } from '#core/components/tag/tag';
import { PhaseEntityList } from './phase-entity-list';
import { PhaseHeader, TopLevelNavLink } from './styled-components';

import type { PhaseConfig } from '#core/types/common/studio-types';
import type { NavLink } from './types';

interface Props {
  phases: PhaseConfig[];
  projectId: string;
  topLevelLinks?: NavLink[];
}

export function MenuDrawer({ phases, projectId, topLevelLinks }: Props) {
  const [css, theme] = useStyletron();
  const [isMenuOpen, setIsMenuOpen] = useState(false);

  return (
    <>
      <Button
        onClick={() => setIsMenuOpen(true)}
        kind={KIND.secondary}
        size={SIZE.mini}
        shape={SHAPE.pill}
        startEnhancer={() => <Icon name="menu" title="" />}
        overrides={{
          BaseButton: {
            style: {
              paddingLeft: theme.sizing.scale500,
              paddingRight: theme.sizing.scale500,
            },
          },
        }}
      >
        Menu
      </Button>
      <Drawer
        isOpen={isMenuOpen}
        autoFocus
        onClose={() => setIsMenuOpen(false)}
        anchor={ANCHOR.left}
        overrides={{
          DrawerContainer: {
            style: { width: '375px' },
          },
          DrawerBody: {
            style: {
              display: 'flex',
              flexDirection: 'column',
              justifyContent: 'flex-start',
              marginLeft: 0,
              marginRight: 0,
              marginBottom: 0,
              marginTop: '44px',
            },
          },
        }}
      >
        <nav
          className={css({ display: 'flex', flexDirection: 'column', gap: theme.sizing.scale400 })}
        >
          {topLevelLinks && topLevelLinks.length > 0 && (
            <div>
              <ul className={css({ listStyle: 'none', margin: 0, padding: 0 })}>
                {topLevelLinks.map((link) => (
                  <li key={link.path}>
                    <TopLevelNavLink to={link.path} onClick={() => setIsMenuOpen(false)}>
                      {link.label}
                      <Icon name="chevronRight" title="" />
                    </TopLevelNavLink>
                  </li>
                ))}
              </ul>
              <hr
                className={css({
                  borderTop: `1px solid ${theme.colors.borderOpaque}`,
                  borderBottom: 0,
                  margin: 0,
                })}
              />
            </div>
          )}
          <span
            className={css({
              ...theme.typography.HeadingXSmall,
              paddingLeft: theme.sizing.scale800,
              paddingRight: theme.sizing.scale800,
            })}
          >
            {projectId}
          </span>
          {phases.map((phase) => (
            <div key={phase.id}>
              <PhaseHeader $disabled={phase.state === 'disabled'}>
                {phase.icon && <Icon name={phase.icon} size="16px" title="" />}
                {phase.name}
                {phase.state === 'comingSoon' && (
                  <Tag size="xSmall" closeable={false}>
                    Coming soon
                  </Tag>
                )}
              </PhaseHeader>
              <PhaseEntityList
                phase={phase}
                projectId={projectId}
                onSelect={() => setIsMenuOpen(false)}
              />
            </div>
          ))}
        </nav>
      </Drawer>
    </>
  );
}
