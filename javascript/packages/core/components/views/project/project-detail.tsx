import { useStyletron } from 'baseui';

import { Box } from '#core/components/box/box';
import { Row } from '#core/components/row/row';
import { DATA_PHASE } from '#core/config/phases/data';
import { DEPLOY_PHASE } from '#core/config/phases/deploy';
import { TRAIN_PHASE } from '#core/config/phases/train';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioQuery } from '#core/hooks/use-studio-query';
import { PhaseCard } from './components/phase-card';
import { SHARED_PROJECT_CELL_CONFIG } from './constants';

import type { Theme } from 'baseui';

export function ProjectDetail() {
  const [css, theme] = useStyletron();
  const { projectId } = useStudioParams('base');
  const { data } = useStudioQuery<{
    project: {
      metadata: {
        name: string;
      };
      spec: {
        description: string;
        owner: {
          owningTeam: string;
        };
        tier: string;
      };
    };
  }>({
    queryName: 'GetProject',
    serviceOptions: {
      name: projectId,
      namespace: projectId,
    },
  });

  return (
    <div
      className={css({
        display: 'flex',
        flexDirection: 'column',
        gridGap: theme.sizing.scale600,
        padding: theme.sizing.scale400,
      })}
    >
      {/* Project Overview */}
      <Box
        description={data?.project?.spec?.description}
        title={data?.project?.metadata?.name}
        overrides={{
          BoxContainer: {
            style: ({ $theme }: { $theme: Theme }) => ({
              backgroundColor: $theme.colors.backgroundSecondary,
            }),
          },
        }}
      >
        <Row items={SHARED_PROJECT_CELL_CONFIG} record={data?.project} />
      </Box>

      <div
        className={css({
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fit, minmax(300px, 1fr))',
          gap: theme.sizing.scale600,
          padding: theme.sizing.scale400,
        })}
      >
        {[DATA_PHASE, TRAIN_PHASE, DEPLOY_PHASE].map((phase, index) => (
          <PhaseCard key={`${phase.name}-${index}`} {...phase} projectId={projectId} />
        ))}
      </div>
    </div>
  );
}
