import { useStyletron } from 'baseui';
import { Button, KIND, SHAPE, SIZE } from 'baseui/button';

import { Box } from '#core/components/box/box';
import { Icon } from '#core/components/icon/icon';
import { Link } from '#core/components/link/link';
import { capitalizeFirstLetter } from '#core/utils/string-utils';

import type { PhaseConfig } from '#core/types/common/studio-types';

export function PhaseCard({ icon, name, description, docUrl, entities }: PhaseConfig) {
  const [css, theme] = useStyletron();

  return (
    <Box
      title={
        <div className={css({ display: 'flex', alignItems: 'center', gap: theme.sizing.scale400 })}>
          <Icon name={icon} size={theme.sizing.scale500} />
          {name}
        </div>
      }
      description={
        description && (
          <div className={css({ display: 'flex', alignItems: 'center' })}>
            {description}
            {docUrl && (
              <Button
                kind={KIND.tertiary}
                onClick={() => window.open(docUrl, '_blank')}
                shape={SHAPE.circle}
                size={SIZE.mini}
              >
                <Icon name="arrowLaunch" title="Learn more" size={theme.sizing.scale500} />
              </Button>
            )}
          </div>
        )
      }
    >
      {entities && entities.length > 0 && (
        <div className={css({ display: 'flex', flexDirection: 'column' })}>
          {entities.map((entity) => {
            return (
              <Link
                key={entity.id}
                href={entity.id}
                overrides={{ Link: { style: theme.typography.ParagraphSmall } }}
              >
                {capitalizeFirstLetter(entity.name)}
              </Link>
            );
          })}
        </div>
      )}
      <Button
        kind={KIND.secondary}
        onClick={() => {
          console.log('Navigate to phase');
        }}
        shape={SHAPE.circle}
        overrides={{ BaseButton: { style: { marginTop: 'auto' } } }}
      >
        <Icon name="chevronRight" size={theme.sizing.scale700} />
      </Button>
    </Box>
  );
}
