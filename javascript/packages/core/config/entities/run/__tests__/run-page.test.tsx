import { render, screen } from '@testing-library/react';

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
