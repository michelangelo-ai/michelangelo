import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { PIPELINE_ENTITY_CONFIG } from '#core/config/entities/pipeline/pipeline';
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
      entities: [PIPELINE_ENTITY_CONFIG],
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
