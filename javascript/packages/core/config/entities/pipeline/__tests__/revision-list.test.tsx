import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { PIPELINE_ENTITY_CONFIG } from '#core/config/entities/pipeline/pipeline';
import { PhaseListRoute } from '#core/router/phase-list-route';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { getInterpolationProviderWrapper } from '#core/test/wrappers/get-interpolation-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import {
  createQueryMockRouter,
  getServiceProviderWrapper,
} from '#core/test/wrappers/get-service-provider-wrapper';
import { getSnackbarProviderWrapper } from '#core/test/wrappers/get-snackbar-provider-wrapper';

describe('PIPELINE_ENTITY_CONFIG: Revisions list variant', () => {
  async function switchToRevisions(user: ReturnType<typeof userEvent.setup>) {
    await user.click(await screen.findByRole('option', { name: 'Revisions' }));
  }

  it('shows the same action menu (Run, Run trigger, Delete) as the Pipelines list', async () => {
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
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test/train/pipelines' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            ListPipeline: { pipelineList: { items: [] } },
            ListRevision: {
              revisionList: {
                items: [
                  {
                    metadata: {
                      name: 'pipeline-eval-pipeline-3f2a1b9c0d4e',
                      namespace: 'ma-dev-test',
                    },
                    spec: {
                      baseResource: { name: 'eval-pipeline', namespace: 'ma-dev-test' },
                      revisionId: '3f2a1b9c0d4e5f6a7b8c',
                      owner: { name: 'me' },
                      gitCommit: { branch: 'main' },
                      content: {
                        spec: {
                          manifest: {
                            triggerMap: {
                              nightly: {
                                triggerType: { case: 'cronSchedule', value: { cron: '0 2 * * *' } },
                              },
                            },
                          },
                        },
                      },
                    },
                  },
                ],
              },
            },
          }),
        }),
        getSnackbarProviderWrapper(),
      ])
    );

    await switchToRevisions(user);
    await user.click(await screen.findByRole('button', { name: 'Actions' }));

    expect(screen.getByRole('option', { name: 'Run' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'Run trigger' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'Delete' })).toBeInTheDocument();

    await user.keyboard('{Escape}');
  });

  it('uses specified Revision when creating pipeline run', async () => {
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
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test/train/pipelines' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            ListPipeline: { pipelineList: { items: [] } },
            ListRevision: {
              revisionList: {
                items: [
                  {
                    metadata: {
                      name: 'pipeline-eval-pipeline-3f2a1b9c0d4e',
                      namespace: 'ma-dev-test',
                    },
                    spec: {
                      baseResource: { name: 'eval-pipeline', namespace: 'ma-dev-test' },
                      revisionId: '3f2a1b9c0d4e5f6a7b8c',
                      owner: { name: 'me' },
                      gitCommit: { branch: 'main' },
                    },
                  },
                ],
              },
            },
          }),
        }),
        getSnackbarProviderWrapper(),
      ])
    );

    await switchToRevisions(user);
    await user.click(await screen.findByRole('button', { name: 'Actions' }));
    await user.click(await screen.findByRole('option', { name: 'Run' }));

    const dialog = await screen.findByRole('dialog', { name: 'Start new pipeline run' });
    expect(
      await within(dialog).findByDisplayValue('pipeline-eval-pipeline-3f2a1b9c0d4e')
    ).toHaveAttribute('id', 'spec.revision.name');
  });

  it('disables Run trigger when the pipeline revision has no triggers', async () => {
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
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test/train/pipelines' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            ListPipeline: { pipelineList: { items: [] } },
            ListRevision: {
              revisionList: {
                items: [
                  {
                    metadata: {
                      name: 'pipeline-eval-pipeline-3f2a1b9c0d4e',
                      namespace: 'ma-dev-test',
                    },
                    spec: {
                      baseResource: { name: 'eval-pipeline', namespace: 'ma-dev-test' },
                      revisionId: '3f2a1b9c0d4e5f6a7b8c',
                      owner: { name: 'me' },
                      gitCommit: { branch: 'main' },
                      content: { spec: { manifest: {} } },
                    },
                  },
                ],
              },
            },
          }),
        }),
        getSnackbarProviderWrapper(),
      ])
    );

    await switchToRevisions(user);
    await user.click(await screen.findByRole('button', { name: 'Actions' }));
    await user.hover(await screen.findByRole('option', { name: 'Run trigger' }));
    expect(await screen.findByText('No triggers defined for this pipeline')).toBeInTheDocument();
  });
});
