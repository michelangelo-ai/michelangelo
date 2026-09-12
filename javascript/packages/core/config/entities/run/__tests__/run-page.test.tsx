import { render, screen, within } from '@testing-library/react';

import { TRAIN_PHASE } from '#core/config/phases/train';
import { EntityDetailRoute } from '#core/router/entity-detail-route';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import {
  createQueryMockRouter,
  getServiceProviderWrapper,
} from '#core/test/wrappers/get-service-provider-wrapper';

describe('Run detail page', () => {
  describe('information tab', () => {
    const buildRun = (overrides: Record<string, unknown> = {}) => ({
      metadata: {
        name: 'run-1',
        creationTimestamp: { seconds: '1700000000' },
        labels: { 'michelangelo/environment': 'development' },
      },
      spec: {
        actor: { name: 'jsmith' },
        pipeline: { name: 'prediction-pipeline' },
      },
      status: {
        state: 3,
        steps: [
          {
            name: 'Execute Workflow',
            displayName: 'Execute Workflow',
            state: 3,
            startTime: { seconds: '1700000010' },
            endTime: { seconds: '1700003186' },
            logUrl: 'https://workflow.example.com/run-1',
          },
        ],
      },
      ...overrides,
    });

    /** The JSON editor rendered inside the Box whose title is `title`. */
    const findEditorTitled = (title: string) => {
      let node: HTMLElement | null = screen.getByText(title);
      while (node && !node.querySelector('[role="textbox"]')) {
        node = node.parentElement;
      }
      if (!node) {
        throw new Error(`no editor found under the "${title}" box`);
      }
      return within(node).getByRole('textbox');
    };

    it('renders the workflow log link, status indicators, and environment', async () => {
      render(
        <EntityDetailRoute phases={{ train: TRAIN_PHASE }} />,
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper({
            location: '/myproject/train/runs/run-1/information',
          }),
          getServiceProviderWrapper({
            request: createQueryMockRouter({ GetPipelineRun: { pipelineRun: buildRun() } }),
          }),
        ])
      );

      const logLink = await screen.findByRole('link', { name: 'Michelangelo pipeline run logs' });
      expect(logLink).toHaveAttribute('href', 'https://workflow.example.com/run-1');

      // 1700003186 - 1700000000 = 3186s = 53 minutes 6 seconds
      expect(screen.getByLabelText('Duration')).toHaveValue('53 minutes 6 seconds');
      const timestamp = screen.getByLabelText<HTMLInputElement>('Execution Timestamp');
      // Local-timezone rendering: fix the date, leave time-of-day and zone name open.
      expect(timestamp.value).toMatch(/^2023\/11\/1[45] \d{2}:\d{2}:\d{2} \(.+\)$/);
      expect(screen.getByLabelText('Environment')).toHaveValue('development');
      // Manual run: no parameter-id label, so the field renders empty.
      expect(screen.getByLabelText('Parameter ID')).toHaveValue('');
      // Successful run with no manifest snapshot or input: the conditional blocks stay hidden.
      expect(screen.queryByText('Message')).not.toBeInTheDocument();
      expect(screen.queryByText('Content')).not.toBeInTheDocument();
      expect(screen.queryByText('Input')).not.toBeInTheDocument();
    });

    it('shows the error message only for a run that failed with one', async () => {
      render(
        <EntityDetailRoute phases={{ train: TRAIN_PHASE }} />,
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper({
            location: '/myproject/train/runs/run-1/information',
          }),
          getServiceProviderWrapper({
            request: createQueryMockRouter({
              GetPipelineRun: {
                pipelineRun: buildRun({
                  status: {
                    state: 5,
                    steps: [],
                    errorMessage: 'Task train failed:\nOOMKilled',
                  },
                }),
              },
            }),
          }),
        ])
      );

      expect(await screen.findByText('Message')).toBeInTheDocument();
      expect(screen.getByLabelText('Error message')).toHaveValue('Task train failed:\nOOMKilled');
    });

    it('prefers the scheduled execution timestamp label over creation time', async () => {
      render(
        <EntityDetailRoute phases={{ train: TRAIN_PHASE }} />,
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper({
            location: '/myproject/train/runs/run-1/information',
          }),
          getServiceProviderWrapper({
            request: createQueryMockRouter({
              GetPipelineRun: {
                pipelineRun: buildRun({
                  metadata: {
                    name: 'run-1',
                    creationTimestamp: { seconds: '1700000000' },
                    labels: {
                      'michelangelo/environment': 'production',
                      // 2023-07-22T05:46:40Z — a backfill slot well before the object was created.
                      'pipelinerun.michelangelo/execution-timestamp': '1690004800',
                      'pipelinerun.michelangelo/parameter-id': 'daily-01',
                    },
                  },
                }),
              },
            }),
          }),
        ])
      );

      const timestamp = await screen.findByLabelText<HTMLInputElement>('Execution Timestamp');
      // Local-timezone rendering: fix the date, leave time-of-day and zone name open.
      expect(timestamp.value).toMatch(/^2023\/07\/2[12] \d{2}:\d{2}:\d{2} \(.+\)$/);
      expect(screen.getByLabelText('Parameter ID')).toHaveValue('daily-01');
    });

    it('renders the manifest content and run input as read-only JSON', async () => {
      render(
        <EntityDetailRoute phases={{ train: TRAIN_PHASE }} />,
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper({
            location: '/myproject/train/runs/run-1/information',
          }),
          getServiceProviderWrapper({
            request: createQueryMockRouter({
              GetPipelineRun: {
                pipelineRun: buildRun({
                  spec: {
                    actor: { name: 'jsmith' },
                    pipeline: { name: 'prediction-pipeline' },
                    // Raw protobuf Struct, as the rpc layer delivers `spec.input`.
                    input: {
                      fields: {
                        learning_rate: { numberValue: 0.01 },
                        dataset: { stringValue: 'boston' },
                      },
                    },
                  },
                  status: {
                    state: 3,
                    steps: [],
                    sourcePipeline: {
                      pipeline: {
                        spec: {
                          manifest: {
                            type: 1,
                            content: {
                              typeUrl: 'type.googleapis.com/michelangelo.PredictionPipelineConf',
                              value: { meta: { workflow_version: 'v2' } },
                            },
                          },
                        },
                      },
                    },
                  },
                }),
              },
            }),
          }),
        ])
      );

      await screen.findByText('Content');
      // The CodeMirror editor exposes its content as a readonly textbox.
      expect(findEditorTitled('Content')).toHaveTextContent('"workflow_version": "v2"');
      const inputEditor = findEditorTitled('Input');
      expect(inputEditor).toHaveTextContent('"learning_rate": 0.01');
      expect(inputEditor).toHaveTextContent('"dataset": "boston"');
    });

    it('links a resumed run back to its source run', async () => {
      render(
        <EntityDetailRoute phases={{ train: TRAIN_PHASE }} />,
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper({
            location: '/myproject/train/runs/run-1/information',
          }),
          getServiceProviderWrapper({
            request: createQueryMockRouter({
              GetPipelineRun: {
                pipelineRun: buildRun({
                  spec: {
                    actor: { name: 'jsmith' },
                    pipeline: { name: 'prediction-pipeline' },
                    resume: { pipelineRun: { name: 'run-0', namespace: 'myproject' } },
                  },
                }),
              },
            }),
          }),
        ])
      );

      const resumeLink = await screen.findByRole('link', { name: 'Resumed from run-0' });
      expect(resumeLink).toHaveAttribute('href', '/myproject/train/runs/run-0');
    });

    it('omits the resume link and duration for a run that has not produced them', async () => {
      render(
        <EntityDetailRoute phases={{ train: TRAIN_PHASE }} />,
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper({
            location: '/myproject/train/runs/run-1/information',
          }),
          getServiceProviderWrapper({
            request: createQueryMockRouter({
              GetPipelineRun: {
                pipelineRun: buildRun({ status: { state: 1, steps: [] } }),
              },
            }),
          }),
        ])
      );

      expect(await screen.findByLabelText('Duration')).toHaveValue('');
      expect(screen.queryByRole('link', { name: /Resumed from/ })).not.toBeInTheDocument();
    });
  });
});
