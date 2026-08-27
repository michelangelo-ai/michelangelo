import { render, screen } from '@testing-library/react';
import { vi } from 'vitest';

import { TRAIN_PHASE } from '#core/config/phases/train';
import { EntityDetailRoute } from '#core/router/entity-detail-route';
import { PhaseListRoute } from '#core/router/phase-list-route';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import {
  createQueryMockRouter,
  getServiceProviderWrapper,
} from '#core/test/wrappers/get-service-provider-wrapper';
import { getUserProviderWrapper } from '#core/test/wrappers/get-user-provider-wrapper';

describe('Run list page', () => {
  it('renders the column headers in order', async () => {
    render(
      <PhaseListRoute phases={{ train: TRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/train/runs' }),
        getUserProviderWrapper(),
        getServiceProviderWrapper({
          request: vi.fn().mockResolvedValue({ pipelineRunList: { items: [] } }),
        }),
      ])
    );

    const headers = await screen.findAllByRole('columnheader');
    // Trailing columns with no text content are table chrome (e.g. a row-actions column),
    // not a named data column, so they're excluded from the ordering assertion.
    const headerLabels = headers.map((header) => header.textContent).filter(Boolean);
    expect(headerLabels).toEqual([
      'Pipeline run name',
      'Pipeline',
      'Last Updated',
      'Created',
      'Environment',
      'Started by',
      'Triggered by',
      'State',
    ]);
  });

  it('renders Last Updated and Environment values, with fallbacks', async () => {
    render(
      <PhaseListRoute phases={{ train: TRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/train/runs' }),
        getUserProviderWrapper(),
        getServiceProviderWrapper({
          request: vi.fn().mockResolvedValue({
            pipelineRunList: {
              items: [
                {
                  metadata: {
                    name: 'run-with-labels',
                    labels: {
                      'michelangelo/UpdateTimestamp': '1700000000000000',
                      'michelangelo/environment': 'production',
                    },
                    // Distinct from run-without-labels' creationTimestamp below so this row's
                    // Created value (which always reads creationTimestamp) doesn't collide with
                    // the other row's Last Updated fallback (which also reads creationTimestamp).
                    creationTimestamp: { seconds: 1660000000 },
                  },
                  spec: { actor: { name: 'jsmith' }, pipeline: { name: 'prediction-pipeline' } },
                  status: { state: 3 },
                },
                {
                  metadata: {
                    name: 'run-without-labels',
                    creationTimestamp: { seconds: 1650000000 },
                  },
                  spec: { actor: { name: 'jsmith' }, pipeline: { name: 'prediction-pipeline' } },
                  status: { state: 3 },
                },
              ],
            },
          }),
        }),
      ])
    );

    expect(await screen.findByRole('link', { name: 'run-with-labels' })).toBeInTheDocument();
    expect(screen.getByText('Production')).toBeInTheDocument();
    // The UpdateTimestamp label (1700000000 seconds) is used over creationTimestamp.
    expect(screen.getByText('2023/11/14 22:13:20 (UTC)')).toBeInTheDocument();
    // Created always reads creationTimestamp (1660000000 seconds), independent of Last Updated.
    expect(screen.getByText('2022/08/08 23:06:40 (UTC)')).toBeInTheDocument();

    expect(screen.getByRole('link', { name: 'run-without-labels' })).toBeInTheDocument();
    // No UpdateTimestamp label: Last Updated falls back to creationTimestamp (1650000000
    // seconds), the same value Created reads directly — both cells render this text.
    expect(screen.getAllByText('2022/04/15 05:20:00 (UTC)')).toHaveLength(2);
    // No environment label: renders no Environment text at all.
    expect(screen.queryByText('Development')).not.toBeInTheDocument();
    expect(screen.queryByText('Testing')).not.toBeInTheDocument();
  });
});

describe('Run detail page', () => {
  describe('configuration tab', () => {
    const buildRun = (overrides: Record<string, unknown> = {}) => ({
      metadata: { name: 'run-1', creationTimestamp: { seconds: 1700000000 } },
      spec: {
        actor: { name: 'jsmith' },
        pipeline: { name: 'prediction-pipeline' },
      },
      status: {
        state: 3,
        sourcePipeline: {
          pipeline: {
            spec: {
              manifest: {
                // The generated proto client decodes enum fields to their numeric
                // discriminant (PIPELINE_MANIFEST_TYPE_YAML = 1), not the enum's string name.
                type: 1,
                filePath: 'python/examples/boston/pipeline.yaml',
                content: {
                  typeUrl: 'type.googleapis.com/michelangelo.PredictionPipelineConf',
                  value: {
                    meta: {
                      workflow_version: 'v2',
                      app: 'data.michelangelo.asl.app.prediction.PredictionPipeline',
                    },
                    triggers: {
                      'daily-01': { cron: '0 01 * * *' },
                    },
                  },
                },
              },
            },
          },
        },
      },
      ...overrides,
    });

    const buildConfigurationTabWrapper = (run: object) =>
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({
          location: '/myproject/train/runs/run-1/configuration',
        }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({ GetPipelineRun: { pipelineRun: run } }),
        }),
      ]);

    it('renders the manifest configuration as JSON', async () => {
      const run = buildRun();
      render(
        <EntityDetailRoute phases={{ train: TRAIN_PHASE }} />,
        buildConfigurationTabWrapper(run)
      );

      expect(await screen.findByText('General')).toBeInTheDocument();
      expect(screen.getByText('Manifest content')).toBeInTheDocument();
      // The CodeMirror editor exposes its content as a readonly textbox.
      const editor = await screen.findByRole('textbox');
      expect(editor).toHaveTextContent('"workflow_version": "v2"');
      expect(editor).toHaveTextContent('"cron": "0 01 * * *"');
    });

    it('falls back to the inline dev-run pipeline spec before the source pipeline is resolved', async () => {
      const run = buildRun({
        spec: {
          actor: { name: 'jsmith' },
          pipelineSpec: {
            manifest: {
              type: 1,
              content: {
                typeUrl: 'type.googleapis.com/michelangelo.PredictionPipelineConf',
                value: {
                  meta: {
                    workflow_version: 'v2',
                    app: 'data.michelangelo.asl.app.prediction.PredictionPipeline',
                  },
                  triggers: {
                    'daily-01': { cron: '0 01 * * *' },
                  },
                },
              },
            },
          },
        },
        status: { state: 1 },
      });
      render(
        <EntityDetailRoute phases={{ train: TRAIN_PHASE }} />,
        buildConfigurationTabWrapper(run)
      );

      expect(await screen.findByText('General')).toBeInTheDocument();
      expect(await screen.findByRole('textbox')).toHaveTextContent('"workflow_version": "v2"');
    });

    it('shows an empty state when the manifest has no configuration content', async () => {
      const run = buildRun({
        status: {
          state: 3,
          sourcePipeline: {
            pipeline: {
              spec: {
                manifest: { type: 3, uniflowTar: 's3://default/bert_local.tar' },
              },
            },
          },
        },
      });
      render(
        <EntityDetailRoute phases={{ train: TRAIN_PHASE }} />,
        buildConfigurationTabWrapper(run)
      );

      expect(await screen.findByText('No configuration available')).toBeInTheDocument();
      expect(screen.queryByText(/workflow_version/)).not.toBeInTheDocument();
    });
  });
});
