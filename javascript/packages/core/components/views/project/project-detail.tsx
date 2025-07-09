import { useStyletron } from 'baseui';

import { Box } from '#core/components/box/box';
import { Row } from '#core/components/row/row';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioQuery } from '#core/hooks/use-studio-query';
import { PIPELINE_CELL_CONFIG, SHARED_PROJECT_CELL_CONFIG } from './constants';

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

  const pipelines = useStudioQuery<{
    pipelineList: {
      items: Array<{
        metadata: {
          name: string;
        };
        spec: {
          type: string;
        };
        status: {
          state: string;
        };
      }>;
    };
  }>({
    queryName: 'ListPipeline',
    serviceOptions: {
      name: projectId,
      namespace: projectId,
    },
  });

  return (
    <div className={css({ gap: theme.sizing.scale400, display: 'flex', flexDirection: 'column' })}>
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
        <Row items={SHARED_PROJECT_CELL_CONFIG} record={data?.project} />
      </Box>

      {pipelines?.data?.pipelineList.items.map((item, index) => (
        <Row
          overrides={{
            RowItemContainer: {
              style: {
                width: '200px',
              },
            },
          }}
          key={item.metadata.name}
          record={item}
          items={[
            { id: 'metadata.name', label: 'Name', url: item.metadata.name },
            ...PIPELINE_CELL_CONFIG,
          ].map((cell) => ({
            ...cell,
            ...(index > 0 && { label: undefined }),
          }))}
        />
      ))}
    </div>
  );
}
