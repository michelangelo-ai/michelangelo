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

describe('Run list page', () => {
  it('renders column headers in the expected order', async () => {
    render(
      <PhaseListRoute phases={{ train: TRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/train/runs' }),
        getServiceProviderWrapper({
          request: vi.fn().mockResolvedValue({ pipelineRunList: { items: [] } }),
        }),
      ])
    );

    const expectedHeaders = [
      'Pipeline run name',
      'Pipeline',
      'Last Updated',
      'Created',
      'Environment',
      'Started by',
      'State',
    ];

    const firstHeader = await screen.findByRole('columnheader', { name: expectedHeaders[0] });
    expect(firstHeader).toBeInTheDocument();

    for (const label of expectedHeaders.slice(1)) {
      expect(screen.getByRole('columnheader', { name: label })).toBeInTheDocument();
    }

    const allHeaders = screen.getAllByRole('columnheader');
    const headerLabels = allHeaders.map((h) => h.textContent).filter(Boolean);
    expect(headerLabels).toEqual(expectedHeaders);
  });

  it('renders environment labels and run data for list rows', async () => {
    render(
      <PhaseListRoute phases={{ train: TRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/train/runs' }),
        getServiceProviderWrapper({
          request: vi.fn().mockResolvedValue({
            pipelineRunList: {
              items: [
                {
                  metadata: {
                    name: 'train-fraud-run',
                    labels: { 'michelangelo/environment': 'production' },
                    creationTimestamp: { seconds: 1700000000 },
                  },
                  spec: {
                    description: 'training run for fraud detection',
                    pipeline: { name: 'fraud-pipeline' },
                    revision: { name: 'rev-1' },
                    actor: { name: 'alice' },
                  },
                  status: { state: 3 },
                },
                {
                  metadata: {
                    name: 'eval-demand-run',
                    labels: { 'michelangelo/environment': 'development' },
                    creationTimestamp: { seconds: 1700000000 },
                  },
                  spec: {
                    description: 'eval run for demand model',
                    pipeline: { name: 'demand-pipeline' },
                    revision: { name: 'rev-2' },
                    actor: { name: 'bob' },
                  },
                  status: { state: 2 },
                },
                {
                  metadata: {
                    name: 'test-segment-run',
                    creationTimestamp: { seconds: 1700000000 },
                  },
                  spec: {
                    description: 'segment run',
                    pipeline: { name: 'segment-pipeline' },
                    actor: { name: 'carol' },
                  },
                  status: { state: 5 },
                },
              ],
            },
          }),
        }),
      ])
    );

    expect(await screen.findByRole('link', { name: 'train-fraud-run' })).toBeInTheDocument();
    expect(screen.getByText('Production')).toBeInTheDocument();
    expect(screen.getByText('fraud-pipeline')).toBeInTheDocument();
    expect(screen.getByText('alice')).toBeInTheDocument();

    expect(screen.getByRole('link', { name: 'eval-demand-run' })).toBeInTheDocument();
    expect(screen.getByText('Development')).toBeInTheDocument();

    expect(screen.getByRole('link', { name: 'test-segment-run' })).toBeInTheDocument();
  });
});

describe('Run detail page', () => {
  describe('configuration tab', () => {
    const buildManifestContent = () => ({
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
    });

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
                content: buildManifestContent(),
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
            manifest: { type: 1, content: buildManifestContent() },
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
