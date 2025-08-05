import { useStyletron } from 'baseui';
import { Card } from 'baseui/card';
import { HeadingMedium } from 'baseui/typography';

import { Box } from '#core/components/box/box';
import { Row } from '#core/components/row/row';
import { useLocalStorageTableState } from '#core/components/table/plugins/state-persistence/use-local-storage-table-state';
import { Table } from '#core/components/table/table';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioQuery } from '#core/hooks/use-studio-query';
import { PhaseCard } from './components/phase-list/phase-card';
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

  const pipelineTableState = useLocalStorageTableState({
    tableSettingsId: 'pipelines',
  });

  const pipelineRunTableState = useLocalStorageTableState({
    tableSettingsId: 'pipeline-runs',
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

      {/* Phase Card Demo */}
      <div
        className={css({
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fit, minmax(300px, 1fr))',
          gap: theme.sizing.scale600,
        })}
      >
        <PhaseCard
          icon="database"
          name="Prepare & Analyze Data"
          description="Create data pipelines and analyze your datasets"
          docUrl="https://example.com/docs/data"
          entities={[
            { id: 'pipelines', name: 'pipelines' },
            { id: 'datasources', name: 'data sources' },
            { id: 'runs', name: 'runs' },
          ]}
        />

        <PhaseCard
          icon="chartLine"
          name="Train & Evaluate"
          description="Train machine learning models and evaluate their performance"
          docUrl="https://example.com/docs/train"
          entities={[
            { id: 'pipelines', name: 'pipelines' },
            { id: 'runs', name: 'runs' },
            { id: 'models', name: 'trained models' },
            { id: 'evaluations', name: 'evaluations' },
            { id: 'notebooks', name: 'notebooks' },
          ]}
        />
      </div>

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

        <Table
          data={pipelines?.data?.pipelineList.items ?? []}
          columns={PIPELINE_CELL_CONFIG}
          loading={pipelines.isLoading}
          state={pipelineTableState}
        />
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

        <Table
          data={pipelineRuns?.data?.pipelineRunList.items ?? []}
          columns={PIPELINE_RUN_CELL_CONFIG}
          loading={pipelineRuns.isLoading}
          state={pipelineRunTableState}
        />
      </Card>
    </div>
  );
}
