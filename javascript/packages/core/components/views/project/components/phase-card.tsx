import { useNavigate } from 'react-router-dom-v5-compat';
import { useStyletron } from 'baseui';
import { Button, KIND, SHAPE, SIZE } from 'baseui/button';

import { Box } from '#core/components/box/box';
import { Icon } from '#core/components/icon/icon';
import { Link } from '#core/components/link/link';
import { capitalizeFirstLetter } from '#core/utils/string-utils';

import type { PhaseConfig } from '#core/types/common/studio-types';

export function PhaseCard(props: PhaseConfig & { projectId: string }) {
  const { id, icon, name, description, docUrl, state, entities, projectId } = props;
  const navigate = useNavigate();
  const [css, theme] = useStyletron();

  const isPhaseDisabled = state === 'disabled' || state === 'comingSoon';
  const isComingSoon = state === 'comingSoon';

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
                aria-label="Learn more"
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
      {isComingSoon ? (
        <div
          className={css({
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            flex: '1',
            color: theme.colors.contentTertiary,
          })}
        >
          Coming soon
        </div>
      ) : (
        <div className={css({ display: 'flex', flexDirection: 'column' })}>
          {entities.map((entity) => {
            const isEntityDisabled = isPhaseDisabled || entity.state === 'disabled';

            if (isEntityDisabled) {
              return (
                <span
                  key={entity.id}
                  className={css({
                    ...theme.typography.ParagraphSmall,
                    cursor: 'default',
                    color: theme.colors.contentTertiary,
                  })}
                >
                  {capitalizeFirstLetter(entity.name)}
                </span>
              );
            }

            return (
              <Link
                key={entity.id}
                href={`/${projectId}/${id}/${entity.id}`}
                overrides={{ Link: { style: theme.typography.ParagraphSmall } }}
              >
                {capitalizeFirstLetter(entity.name)}
              </Link>
            );
          })}
        </div>
      )}

      {!isPhaseDisabled && entities.length > 0 && (
        <Button
          kind={KIND.secondary}
          onClick={() => {
            const firstEntity = entities.find((entity) => entity.state === 'active') ?? entities[0];
            navigate(`/${projectId}/${id}/${firstEntity.id}`);
          }}
          shape={SHAPE.circle}
          overrides={{ BaseButton: { style: { marginTop: 'auto' } } }}
        >
          <Icon name="chevronRight" size={theme.sizing.scale700} />
        </Button>
      )}
    </Box>
  );
}
