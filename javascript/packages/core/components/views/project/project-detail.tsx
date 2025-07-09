import { useStyletron } from 'baseui';
import { Card } from 'baseui/card';
import { HeadingMedium } from 'baseui/typography';

import { Box } from '#core/components/box/box';
import { Row } from '#core/components/row/row';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioQuery } from '#core/hooks/use-studio-query';
import {
  PIPELINE_CELL_CONFIG,
  PIPELINE_RUN_CELL_CONFIG,
  SHARED_PROJECT_CELL_CONFIG,
} from './constants';

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
      namespace: projectId,
    },
  });

  const pipelineRuns = useStudioQuery<{
    pipelineRunList: { items: Array<{ metadata: { name: string } }> };
  }>({
    queryName: 'ListPipelineRun',
    serviceOptions: {
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

      {/* Pipelines Section */}
      <Card
        overrides={{
          Root: {
            style: ({ $theme }: { $theme: Theme }) => ({
              borderRadius: $theme.borders.radius400,
              backgroundColor: $theme.colors.backgroundPrimary,
            }),
          },
        }}
      >
        <div className={css({ marginBottom: theme.sizing.scale600 })}>
          <HeadingMedium margin={0}>Pipelines</HeadingMedium>
        </div>

        {(pipelines?.data?.pipelineList.items ?? []).length > 0 ? (
          <div
            className={css({
              display: 'flex',
              flexDirection: 'column',
              gridGap: theme.sizing.scale300,
            })}
          >
            {(pipelines?.data?.pipelineList.items ?? []).map((item, index) => (
              <Row
                overrides={{
                  RowItemContainer: {
                    style: {
                      minWidth: '250px',
                    },
                  },
                }}
                key={item.metadata.name}
                record={item}
                items={[
                  {
                    id: 'metadata.name',
                    label: 'Name',
                    url: `/project/${projectId}/pipelines/${item.metadata.name}`,
                  },
                  ...PIPELINE_CELL_CONFIG,
                ].map((cell) => ({
                  ...cell,
                  ...(index > 0 && { label: undefined }),
                }))}
              />
            ))}
          </div>
        ) : (
          <div className={css({ color: theme.colors.contentSecondary })}>
            <em>No pipelines found</em>
          </div>
        )}
      </Card>

      {/* Pipeline Runs Section */}
      <Card
        overrides={{
          Root: {
            style: ({ $theme }: { $theme: Theme }) => ({
              borderRadius: $theme.borders.radius400,
              backgroundColor: $theme.colors.backgroundPrimary,
            }),
          },
        }}
      >
        <div className={css({ marginBottom: theme.sizing.scale600 })}>
          <HeadingMedium margin={0}>Pipeline Runs</HeadingMedium>
        </div>

        {(pipelineRuns?.data?.pipelineRunList.items ?? []).length > 0 ? (
          <div
            className={css({
              display: 'flex',
              flexDirection: 'column',
              gridGap: theme.sizing.scale300,
            })}
          >
            {(pipelineRuns?.data?.pipelineRunList.items ?? []).map((item, index) => (
              <Row
                overrides={{
                  RowItemContainer: {
                    style: {
                      minWidth: '250px',
                    },
                  },
                }}
                key={item.metadata.name}
                record={item}
                items={[
                  {
                    id: 'metadata.name',
                    label: 'Name',
                    url: `/project/${projectId}/runs/${item.metadata.name}`,
                  },
                  ...PIPELINE_RUN_CELL_CONFIG,
                ].map((cell) => ({
                  ...cell,
                  ...(index > 0 && { label: undefined }),
                }))}
              />
            ))}
          </div>
        ) : (
          <div className={css({ color: theme.colors.contentSecondary })}>
            <em>No pipeline runs found</em>
          </div>
        )}
      </Card>
    </div>
  );
}
