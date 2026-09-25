import { render, screen, waitFor, within } from '@testing-library/react';
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

// baseui's dialog dismiss button renders an icon component it internally names "Delete"
// (unrelated to our action) with no aria-label, so it collides with the confirm button's
// accessible name. Scope to the button-dock footer to find the real submit button.
function getSubmitButton(dialog: HTMLElement) {
  const footer = dialog.querySelector('[data-baseweb="button-dock"]');
  if (!footer) throw new Error('Expected dialog to render a button-dock footer');
  return within(footer as HTMLElement).getByRole('button', { name: 'Delete' });
}

describe('PIPELINE_ENTITY_CONFIG: Revisions list variant', () => {
  describe('list view', () => {
    function renderRevisionsList(mockRequest: ReturnType<typeof createQueryMockRouter>) {
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
          getServiceProviderWrapper({ request: mockRequest }),
          getSnackbarProviderWrapper(),
        ])
      );
    }

    async function switchToRevisions(user: ReturnType<typeof userEvent.setup>) {
      await user.click(await screen.findByRole('option', { name: 'Revisions' }));
    }

    it('shows the same action menu (Run, Run trigger, Delete) as the Pipelines list', async () => {
      const user = userEvent.setup();
      renderRevisionsList(
        createQueryMockRouter({
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
        })
      );

      await switchToRevisions(user);
      await user.click(await screen.findByRole('button', { name: 'Actions' }));

      expect(screen.getByRole('option', { name: 'Run' })).toBeInTheDocument();
      expect(screen.getByRole('option', { name: 'Run trigger' })).toBeInTheDocument();
      expect(screen.getByRole('option', { name: 'Delete' })).toBeInTheDocument();

      // Close the menu rather than leaving it open at test end — an open StatefulPopover
      // leaks its layer/focus-lock state into whichever test runs next.
      await user.keyboard('{Escape}');
    });

    it('deletes the pipeline the revision belongs to, not just the revision row', async () => {
      const user = userEvent.setup();
      const mockRequest = createQueryMockRouter({
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
        DeletePipeline: {},
      });
      renderRevisionsList(mockRequest);

      await switchToRevisions(user);
      await user.click(await screen.findByRole('button', { name: 'Actions' }));
      await user.click(await screen.findByRole('option', { name: 'Delete' }));

      const dialog = await screen.findByRole('dialog', { name: 'Delete Pipeline' });
      expect(within(dialog).getByText(/Delete pipeline/)).toHaveTextContent(
        /Delete pipeline eval-pipeline\? This action cannot be undone\./
      );

      await user.click(getSubmitButton(dialog));

      // The row is the PipelineRevision, not the Pipeline — mutation middleware retargets
      // `metadata` at `spec.baseResource` (the Revision's pointer to the Pipeline it snapshots)
      // before sending, so the whole pipeline is deleted, not just this revision.
      await waitFor(() => {
        expect(mockRequest).toHaveBeenCalledWith(
          'DeletePipeline',
          expect.objectContaining({
            metadata: { name: 'eval-pipeline', namespace: 'ma-dev-test' },
          }),
          {}
        );
      });

      // Navigation is a success operation that runs after the mutation resolves, i.e.
      // asynchronously relative to the waitFor above — assert on it with findByText,
      // not a synchronous getByText, so the test doesn't race the navigation.
      expect(
        await screen.findByText(/Current pathname: \/ma-dev-test\/train\/pipelines/)
      ).toBeInTheDocument();
    });

    it('pins Run to the specific revision being viewed, not the pipeline latest pointer', async () => {
      const user = userEvent.setup();
      renderRevisionsList(
        createQueryMockRouter({
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
        })
      );

      await switchToRevisions(user);
      await user.click(await screen.findByRole('button', { name: 'Actions' }));
      await user.click(await screen.findByRole('option', { name: 'Run' }));

      const dialog = await screen.findByRole('dialog', { name: 'Start new pipeline run' });
      expect(
        await within(dialog).findByDisplayValue('pipeline-eval-pipeline-3f2a1b9c0d4e')
      ).toHaveAttribute('id', 'spec.revision.name');
    });

    it('disables Run trigger when the revision wraps a pipeline with no triggers', async () => {
      const user = userEvent.setup();
      renderRevisionsList(
        createQueryMockRouter({
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
        })
      );

      await switchToRevisions(user);
      await user.click(await screen.findByRole('button', { name: 'Actions' }));
      await user.hover(await screen.findByRole('option', { name: 'Run trigger' }));
      expect(await screen.findByText('No triggers defined for this pipeline')).toBeInTheDocument();
    });
  });
});
