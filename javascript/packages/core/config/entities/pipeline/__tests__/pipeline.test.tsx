import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { PIPELINE_ENTITY_CONFIG } from '#core/config/entities/pipeline/pipeline';
import {
  CRITERION_OPERATOR_EQUAL,
  PIPELINE_RUN_PIPELINE_NAME_FIELD,
} from '#core/config/entities/pipeline/shared';
import { PipelineRunState } from '#core/config/entities/run/types';
import { TRIGGER_ENTITY_CONFIG } from '#core/config/entities/trigger/trigger';
import { EntityDetailRoute } from '#core/router/entity-detail-route';
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

import type { PhaseConfig } from '#core/types/common/studio-types';

function buildPipeline() {
  return {
    metadata: { name: 'eval-pipeline', namespace: 'ma-dev-test' },
    spec: { owner: { name: 'me' } },
  };
}

function buildTestPhases(): Record<string, PhaseConfig> {
  return {
    train: {
      id: 'train',
      icon: 'train',
      name: 'Train',
      state: 'active',
      entities: [PIPELINE_ENTITY_CONFIG, TRIGGER_ENTITY_CONFIG],
    },
  };
}

// baseui's dialog dismiss button renders an icon component it internally names "Delete"
// (unrelated to our action) with no aria-label, so it collides with the confirm button's
// accessible name. Scope to the button-dock footer to find the real submit button.
function getSubmitButton(dialog: HTMLElement) {
  const footer = dialog.querySelector('[data-baseweb="button-dock"]');
  if (!footer) throw new Error('Expected dialog to render a button-dock footer');
  return within(footer as HTMLElement).getByRole('button', { name: 'Delete' });
}

describe('PIPELINE_ENTITY_CONFIG: delete action', () => {
  describe('list view', () => {
    function renderList(mockRequest: ReturnType<typeof createQueryMockRouter>) {
      render(
        <PhaseListRoute phases={buildTestPhases()} />,
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

    it('opens dialog to confirm deletion of pipeline and deletes pipeline', async () => {
      const user = userEvent.setup();
      const mockRequest = createQueryMockRouter({
        ListPipeline: { pipelineList: { items: [buildPipeline()] } },
        DeletePipeline: {},
      });

      renderList(mockRequest);

      await user.click(await screen.findByRole('button', { name: 'Actions' }));
      await user.click(await screen.findByRole('option', { name: 'Delete' }));

      const dialog = await screen.findByRole('dialog', { name: 'Delete Pipeline' });
      expect(within(dialog).getByText(/Delete pipeline/)).toHaveTextContent(
        /Delete pipeline eval-pipeline\? This action cannot be undone\./
      );

      await user.click(getSubmitButton(dialog));

      // The record is sent as-is; reshaping it into the flat DeletePipelineRequest
      // ({ name, namespace }) the backend expects happens in the RPC handler (see
      // packages/rpc/handlers.ts's deleteCrd), not in client-side mutation middleware.
      await waitFor(() => {
        expect(mockRequest).toHaveBeenCalledWith('DeletePipeline', buildPipeline(), {});
      });

      // Navigation is a success operation that runs after the mutation resolves, i.e.
      // asynchronously relative to the waitFor above — assert on it with findByText,
      // not a synchronous getByText, so the test doesn't race the navigation.
      expect(
        await screen.findByText(/Current pathname: \/ma-dev-test\/train\/pipelines/)
      ).toBeInTheDocument();
    });

    it('keeps the dialog open and shows the error when delete fails', async () => {
      const user = userEvent.setup();
      const mockRequest = createQueryMockRouter({
        ListPipeline: { pipelineList: { items: [buildPipeline()] } },
        DeletePipeline: new Error('Delete failed'),
      });

      renderList(mockRequest);

      await user.click(await screen.findByRole('button', { name: 'Actions' }));
      await user.click(await screen.findByRole('option', { name: 'Delete' }));
      const dialog = await screen.findByRole('dialog', { name: 'Delete Pipeline' });
      await user.click(getSubmitButton(dialog));

      await within(dialog).findByText(/Test error/);
      expect(screen.getByRole('dialog', { name: 'Delete Pipeline' })).toBeInTheDocument();
    });
  });

  describe('pipeline detail page', () => {
    function renderDetail(mockRequest: ReturnType<typeof createQueryMockRouter>) {
      render(
        <EntityDetailRoute phases={buildTestPhases()} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getErrorProviderWrapper(),
          getIconProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper({ location: '/ma-dev-test/train/pipelines/eval-pipeline/runs' }),
          getServiceProviderWrapper({ request: mockRequest }),
          getSnackbarProviderWrapper(),
        ])
      );
    }

    it('opens dialog to confirm deletion of pipeline, deletes pipeline and navigates to list view', async () => {
      const user = userEvent.setup();
      const mockRequest = createQueryMockRouter({
        GetPipeline: { pipeline: buildPipeline() },
        ListPipelineRun: { pipelineRunList: { items: [] } },
        DeletePipeline: {},
      });

      renderDetail(mockRequest);

      await user.click(await screen.findByRole('button', { name: 'Actions' }));
      await user.click(await screen.findByRole('option', { name: 'Delete' }));

      const dialog = await screen.findByRole('dialog', { name: 'Delete Pipeline' });
      expect(within(dialog).getByText(/Delete pipeline/)).toHaveTextContent(
        /Delete pipeline eval-pipeline\? This action cannot be undone\./
      );

      await user.click(getSubmitButton(dialog));

      await waitFor(() => {
        expect(mockRequest).toHaveBeenCalledWith('DeletePipeline', buildPipeline(), {});
      });

      // Navigation is a success operation that runs after the mutation resolves, i.e.
      // asynchronously relative to the waitFor above — assert on it with findByText,
      // not a synchronous getByText, so the test doesn't race the navigation.
      expect(
        await screen.findByText(/Current pathname: \/ma-dev-test\/train\/pipelines/)
      ).toBeInTheDocument();
    });

    it('keeps the dialog open and shows the error when delete fails', async () => {
      const user = userEvent.setup();
      const mockRequest = createQueryMockRouter({
        GetPipeline: { pipeline: buildPipeline() },
        ListPipelineRun: { pipelineRunList: { items: [] } },
        DeletePipeline: new Error('Delete failed'),
      });

      renderDetail(mockRequest);

      await user.click(await screen.findByRole('button', { name: 'Actions' }));
      await user.click(await screen.findByRole('option', { name: 'Delete' }));
      const dialog = await screen.findByRole('dialog', { name: 'Delete Pipeline' });
      await user.click(getSubmitButton(dialog));

      await within(dialog).findByText(/Test error/);
      expect(screen.getByRole('dialog', { name: 'Delete Pipeline' })).toBeInTheDocument();
    });
  });
});

describe('PIPELINE_ENTITY_CONFIG: Run trigger action', () => {
  describe('pipeline detail page', () => {
    function renderDetail(mockRequest: ReturnType<typeof createQueryMockRouter>) {
      render(
        <EntityDetailRoute phases={buildTestPhases()} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getErrorProviderWrapper(),
          getIconProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper({ location: '/ma-dev-test/train/pipelines/eval-pipeline/runs' }),
          getServiceProviderWrapper({ request: mockRequest }),
          getSnackbarProviderWrapper(),
        ])
      );
    }

    it('is greyed out with an explanatory tooltip when the pipeline declares no triggers', async () => {
      const user = userEvent.setup();
      const mockRequest = createQueryMockRouter({
        GetPipeline: {
          pipeline: { ...buildPipeline(), spec: { ...buildPipeline().spec, manifest: {} } },
        },
        ListPipelineRun: { pipelineRunList: { items: [] } },
      });

      renderDetail(mockRequest);

      const runTriggerButton = await screen.findByRole('button', { name: 'Run trigger' });
      expect(runTriggerButton).toBeDisabled();

      await user.hover(runTriggerButton);
      expect(await screen.findByText('No triggers defined for this pipeline')).toBeInTheDocument();
    });

    it('stays enabled once the pipeline declares a trigger', async () => {
      const user = userEvent.setup();
      const mockRequest = createQueryMockRouter({
        GetPipeline: {
          pipeline: {
            ...buildPipeline(),
            spec: {
              ...buildPipeline().spec,
              manifest: {
                triggerMap: {
                  nightly: { triggerType: { case: 'cronSchedule', value: { cron: '0 2 * * *' } } },
                },
              },
            },
          },
        },
        ListPipelineRun: { pipelineRunList: { items: [] } },
      });

      renderDetail(mockRequest);

      const runTriggerButton = await screen.findByRole('button', { name: 'Run trigger' });
      expect(runTriggerButton).toBeEnabled();

      await user.click(runTriggerButton);
      expect(await screen.findByRole('dialog', { name: 'Run trigger' })).toBeInTheDocument();
    });
  });

  describe('list view', () => {
    /**
     * List rows come from `ListPipeline`, which carries no `spec.manifest` (only a
     * `GetPipeline` response does — see run-trigger-form.tsx). An absent manifest means
     * "unknown," not "no triggers," so the action must not be disabled here.
     */
    it('stays enabled, since trigger data has not loaded', async () => {
      const user = userEvent.setup();
      const mockRequest = createQueryMockRouter({
        ListPipeline: { pipelineList: { items: [buildPipeline()] } },
      });

      render(
        <PhaseListRoute phases={buildTestPhases()} />,
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

      await user.click(await screen.findByRole('button', { name: 'Actions' }));
      expect(await screen.findByRole('option', { name: 'Run trigger' })).not.toHaveAttribute(
        'aria-disabled',
        'true'
      );
    });
  });
});

describe('PIPELINE_DETAIL_CONFIG: runs tab', () => {
  it('filters runs by pipeline name via listOptionsExt criterion', async () => {
    const mockRequest = createQueryMockRouter({
      GetPipeline: { pipeline: { metadata: { name: 'eval-pipeline', namespace: 'ma-dev-test' } } },
      ListPipelineRun: {
        pipelineRunList: {
          items: [
            {
              metadata: { name: 'eval-pipeline-run-1', creationTimestamp: { seconds: 1700000000 } },
              spec: { pipeline: { name: 'eval-pipeline' }, actor: { name: 'me' } },
              status: { state: PipelineRunState.SUCCEEDED },
            },
          ],
        },
      },
    });

    render(
      <EntityDetailRoute phases={buildTestPhases()} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test/train/pipelines/eval-pipeline/runs' }),
        getServiceProviderWrapper({ request: mockRequest }),
        getSnackbarProviderWrapper(),
      ])
    );

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        'ListPipelineRun',
        expect.objectContaining({
          listOptionsExt: {
            operation: {
              criterion: [
                {
                  fieldName: PIPELINE_RUN_PIPELINE_NAME_FIELD,
                  operator: CRITERION_OPERATOR_EQUAL,
                  matchValue: 'eval-pipeline',
                },
              ],
            },
          },
        }),
        {}
      );
    });

    expect(await screen.findByRole('link', { name: 'eval-pipeline-run-1' })).toHaveAttribute(
      'href',
      '/ma-dev-test/train/runs/eval-pipeline-run-1'
    );
  });
});

describe('PIPELINE_ENTITY_CONFIG: Triggers tab', () => {
  it('lists trigger runs scoped to this pipeline, linking each to its detail page', async () => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({
      GetPipeline: { pipeline: buildPipeline() },
      ListTriggerRun: {
        triggerRunList: {
          items: [
            {
              metadata: { name: 'nightly-20240101-120000-abcd1234', namespace: 'ma-dev-test' },
              spec: {
                pipeline: { name: 'eval-pipeline', namespace: 'ma-dev-test' },
                actor: { name: 'me' },
                trigger: { triggerType: { case: 'cronSchedule', value: { cron: '0 2 * * *' } } },
              },
              status: { state: 1 },
            },
            {
              metadata: { name: 'other-20240101-120000-ffff9999', namespace: 'ma-dev-test' },
              spec: {
                pipeline: { name: 'training-pipeline', namespace: 'ma-dev-test' },
                actor: { name: 'me' },
                trigger: { triggerType: { case: 'cronSchedule', value: { cron: '0 3 * * *' } } },
              },
              status: { state: 1 },
            },
          ],
        },
      },
    });

    render(
      <EntityDetailRoute phases={buildTestPhases()} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test/train/pipelines/eval-pipeline/triggers' }),
        getServiceProviderWrapper({ request: mockRequest }),
        getSnackbarProviderWrapper(),
      ])
    );

    expect(
      await screen.findByRole('tab', { name: 'Triggers', selected: true })
    ).toBeInTheDocument();

    // TriggerRun has no pipeline-name label, so the pipeline scoping happens client-side on
    // spec.pipeline.name rather than through a labelSelector on the request.
    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        'ListTriggerRun',
        expect.not.objectContaining({ listOptions: expect.anything() as unknown }),
        {}
      );
    });

    expect(
      screen.queryByRole('link', { name: 'other-20240101-120000-ffff9999' })
    ).not.toBeInTheDocument();

    const nameLink = await screen.findByRole('link', { name: 'nightly-20240101-120000-abcd1234' });
    expect(nameLink).toHaveAttribute(
      'href',
      '/ma-dev-test/train/triggers/nightly-20240101-120000-abcd1234'
    );

    await user.click(nameLink);
    expect(
      await screen.findByText(/Current pathname: \/ma-dev-test\/train\/triggers\/nightly/)
    ).toBeInTheDocument();
  });
});
