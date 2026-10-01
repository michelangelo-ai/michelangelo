import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { PIPELINE_ENTITY_CONFIG } from '#core/config/entities/pipeline/pipeline';
import { PhaseListRoute } from '#core/router/phase-list-route';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { getInterpolationProviderWrapper } from '#core/test/wrappers/get-interpolation-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import {
  createQueryMockRouter,
  getServiceProviderWrapper,
} from '#core/test/wrappers/get-service-provider-wrapper';
import { getUserProviderWrapper } from '#core/test/wrappers/get-user-provider-wrapper';

describe('Pipeline list page', () => {
  describe('Pipelines variant', () => {
    it('renders the column headers in order', async () => {
      render(
        <PhaseListRoute
          phases={{
            train: {
              id: 'train',
              icon: 'train',
              name: 'Train',
              state: 'active',
              entities: [PIPELINE_ENTITY_CONFIG],
            },
          }}
        />,
        buildWrapper([
          getErrorProviderWrapper(),
          getIconProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper({ location: '/ma-dev-test/train/pipelines' }),
          getUserProviderWrapper(),
          getServiceProviderWrapper({
            request: createQueryMockRouter({ ListPipeline: { pipelineList: { items: [] } } }),
          }),
        ])
      );

      const headers = await screen.findAllByRole('columnheader');
      expect(headers.map((header) => header.textContent).filter(Boolean)).toEqual([
        'Name',
        'Last updated',
        'Type',
        'Owner',
        'Branch',
        'State',
      ]);
    });
  });

  describe('Revisions variant', () => {
    it('renders the same column headers in order', async () => {
      const user = userEvent.setup();
      render(
        <PhaseListRoute
          phases={{
            train: {
              id: 'train',
              icon: 'train',
              name: 'Train',
              state: 'active',
              entities: [PIPELINE_ENTITY_CONFIG],
            },
          }}
        />,
        buildWrapper([
          getErrorProviderWrapper(),
          getIconProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper({ location: '/ma-dev-test/train/pipelines' }),
          getUserProviderWrapper(),
          getServiceProviderWrapper({
            request: createQueryMockRouter({
              ListPipeline: { pipelineList: { items: [] } },
              ListRevision: { revisionList: { items: [] } },
            }),
          }),
        ])
      );

      await screen.findAllByRole('columnheader');
      await user.click(screen.getByRole('option', { name: 'Revisions' }));

      const headers = await screen.findAllByRole('columnheader');
      expect(headers.map((header) => header.textContent).filter(Boolean)).toEqual([
        'Name',
        'Last updated',
        'Type',
        'Owner',
        'Branch',
        'State',
      ]);
    });

    it('renders Last updated, Owner, and Branch from the Revision itself', async () => {
      const user = userEvent.setup();
      render(
        <PhaseListRoute
          phases={{
            train: {
              id: 'train',
              icon: 'train',
              name: 'Train',
              state: 'active',
              entities: [PIPELINE_ENTITY_CONFIG],
            },
          }}
        />,
        buildWrapper([
          getErrorProviderWrapper(),
          getIconProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper({ location: '/ma-dev-test/train/pipelines' }),
          getUserProviderWrapper(),
          getServiceProviderWrapper({
            request: createQueryMockRouter({
              ListPipeline: { pipelineList: { items: [] } },
              ListRevision: {
                revisionList: {
                  items: [
                    {
                      metadata: {
                        name: 'pipeline-eval-pipeline-3f2a1b9c0d4e',
                        labels: { 'michelangelo/UpdateTimestamp': '1700000000000000' },
                        creationTimestamp: '2022-08-08T23:06:40Z',
                      },
                      spec: {
                        baseResource: { name: 'eval-pipeline' },
                        revisionId: '3f2a1b9c0d4e5f6a7b8c',
                        owner: { name: 'jsmith' },
                        gitCommit: { branch: 'feature/x' },
                        content: {
                          spec: { type: 'PIPELINE_TYPE_TRAIN' },
                          status: { state: 'PIPELINE_STATE_READY' },
                        },
                      },
                    },
                  ],
                },
              },
            }),
          }),
        ])
      );

      await screen.findAllByRole('columnheader');
      await user.click(screen.getByRole('option', { name: 'Revisions' }));

      expect(await screen.findByRole('link', { name: 'eval-pipeline' })).toBeInTheDocument();
      expect(screen.getByText('2023/11/14 22:13:20 (UTC)')).toBeInTheDocument();
      expect(screen.getByText('jsmith')).toBeInTheDocument();
      expect(screen.getByText('feature/x')).toBeInTheDocument();
    });
  });
});
