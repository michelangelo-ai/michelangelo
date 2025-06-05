import { Box } from '#core/components/box/box';
import { CellType } from '#core/components/cell/constants';
import { Row } from '#core/components/row/row';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioQuery } from '#core/hooks/use-studio-query';

import type { Theme } from 'baseui';

export function ProjectDetail() {
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
    <Box
      description={data?.project?.spec?.description}
      title={data?.project?.metadata?.name}
      overrides={{
        BoxContainer: {
          style: ({ $theme }: { $theme: Theme }) => ({
            backgroundColor: $theme.colors.backgroundSecondary,
            marginTop: $theme.sizing.scale400,
          }),
        },
      }}
    >
      <Row
        items={[
          {
            id: 'metadata.creationTimestamp.seconds',
            label: 'Created',
            type: CellType.DATE,
          },
          {
            id: 'spec.owner.owningTeam',
            label: 'Owner',
          },
          {
            id: 'spec.tier',
            label: 'Tier',
            type: CellType.TAG,
          },
        ]}
        record={data?.project}
      />
    </Box>
  );
}
