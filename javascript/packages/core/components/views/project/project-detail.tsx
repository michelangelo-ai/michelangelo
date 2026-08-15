import { useStyletron } from 'baseui';

import { Box } from '#core/components/box/box';
import { CellType } from '#core/components/cell/constants';
import { Row } from '#core/components/row/row';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioQuery } from '#core/hooks/use-studio-query';
import { PhaseCard } from './components/phase-card';
import { SHARED_PROJECT_CELL_CONFIG } from './constants';

import type { Theme } from 'baseui';
import type { PhaseConfig } from '#core/types/common/studio-types';

export function ProjectDetail({ phases }: { phases: PhaseConfig[] }) {
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
        gitRepo: string;
      };
    };
  }>({
    queryName: 'GetProject',
    serviceOptions: {
      name: projectId,
      namespace: projectId,
    },
  });

  const gitRepo = data?.project?.spec?.gitRepo;
  const overviewCellConfig = gitRepo
    ? [
        ...SHARED_PROJECT_CELL_CONFIG,
        {
          id: 'spec.gitRepo',
          label: 'Source Code',
          type: CellType.LINK,
          url: gitRepo,
          accessor: () => 'Link',
        },
      ]
    : SHARED_PROJECT_CELL_CONFIG;

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
        <Row items={overviewCellConfig} record={data?.project} />
      </Box>

      <div
        className={css({
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fit, minmax(300px, 1fr))',
          gap: theme.sizing.scale600,
          padding: theme.sizing.scale400,
        })}
      >
        {phases.map((phase, index) => (
          <PhaseCard key={`${phase.name}-${index}`} {...phase} projectId={projectId} />
        ))}
      </div>
    </div>
  );
}
