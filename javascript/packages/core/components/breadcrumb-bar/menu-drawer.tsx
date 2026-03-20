import { useState } from 'react';
import { useStyletron } from 'baseui';
import { Button, KIND, SHAPE, SIZE } from 'baseui/button';
import { ANCHOR, Drawer } from 'baseui/drawer';

import { Icon } from '#core/components/icon/icon';
import { Tag } from '#core/components/tag/tag';
import { PhaseEntityList } from './phase-entity-list';
import { PhaseHeader } from './styled-components';

import type { PhaseConfig } from '#core/types/common/studio-types';

export function MenuDrawer({ phases, projectId }: { phases: PhaseConfig[]; projectId: string }) {
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
        <div
          className={css({ display: 'flex', flexDirection: 'column', gap: theme.sizing.scale400 })}
        >
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
        </div>
      </Drawer>
    </>
  );
}
