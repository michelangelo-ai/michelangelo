import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { UseSuccessOperationsTestHarness } from '#core/components/actions/__fixtures__/use-success-operations-test-harness';
import { interpolate } from '#core/interpolation/interpolate';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { createServiceProviderTestContext } from '#core/test/wrappers/get-service-provider-wrapper';
import { getSnackbarProviderWrapper } from '#core/test/wrappers/get-snackbar-provider-wrapper';

describe('useSuccessOperations', () => {
  describe('invalidate', () => {
    it('does nothing when no operations are configured', async () => {
      const user = userEvent.setup();
      const testContext = createServiceProviderTestContext({ request: vi.fn() });

      render(
        <UseSuccessOperationsTestHarness />,
        buildWrapper([
          getBaseProviderWrapper(),
          getRouterWrapper(),
          testContext.wrapper,
          getSnackbarProviderWrapper(),
          getIconProviderWrapper(),
        ])
      );

      const spy = vi.spyOn(testContext.handles.queryClient, 'invalidateQueries');

      await user.click(screen.getByRole('button', { name: 'Run success operations' }));

      expect(spy).not.toHaveBeenCalled();
    });

    it('invalidates a query by name only', async () => {
      const user = userEvent.setup();
      const testContext = createServiceProviderTestContext({ request: vi.fn() });

      render(
        <UseSuccessOperationsTestHarness
          operations={[{ type: 'invalidate', targets: ['ListPipelineRun'] }]}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getRouterWrapper(),
          testContext.wrapper,
          getSnackbarProviderWrapper(),
          getIconProviderWrapper(),
        ])
      );

      const spy = vi.spyOn(testContext.handles.queryClient, 'invalidateQueries');

      await user.click(screen.getByRole('button', { name: 'Run success operations' }));

      expect(spy).toHaveBeenCalledWith({ queryKey: ['ListPipelineRun'] });
    });

    it('invalidates a query by name + serviceOptions', async () => {
      const user = userEvent.setup();
      const testContext = createServiceProviderTestContext({ request: vi.fn() });

      render(
        <UseSuccessOperationsTestHarness
          operations={[
            {
              type: 'invalidate',
              targets: [
                { name: 'GetPipelineRun', serviceOptions: { name: 'run-1', namespace: 'ns' } },
              ],
            },
          ]}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getRouterWrapper(),
          testContext.wrapper,
          getSnackbarProviderWrapper(),
          getIconProviderWrapper(),
        ])
      );

      const spy = vi.spyOn(testContext.handles.queryClient, 'invalidateQueries');

      await user.click(screen.getByRole('button', { name: 'Run success operations' }));

      expect(spy).toHaveBeenCalledWith({
        queryKey: ['GetPipelineRun', { name: 'run-1', namespace: 'ns' }],
      });
    });

    it('processes multiple invalidate targets in order', async () => {
      const user = userEvent.setup();
      const testContext = createServiceProviderTestContext({ request: vi.fn() });

      render(
        <UseSuccessOperationsTestHarness
          operations={[
            {
              type: 'invalidate',
              targets: [
                'ListPipelineRun',
                { name: 'GetPipelineRun', serviceOptions: { name: 'run-1', namespace: 'ns' } },
              ],
            },
          ]}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getRouterWrapper(),
          testContext.wrapper,
          getSnackbarProviderWrapper(),
          getIconProviderWrapper(),
        ])
      );

      const spy = vi.spyOn(testContext.handles.queryClient, 'invalidateQueries');

      await user.click(screen.getByRole('button', { name: 'Run success operations' }));

      expect(spy).toHaveBeenNthCalledWith(1, { queryKey: ['ListPipelineRun'] });
      expect(spy).toHaveBeenNthCalledWith(2, {
        queryKey: ['GetPipelineRun', { name: 'run-1', namespace: 'ns' }],
      });
    });

    it('can invalidate and toast in the same run', async () => {
      const user = userEvent.setup();
      const testContext = createServiceProviderTestContext({ request: vi.fn() });

      render(
        <UseSuccessOperationsTestHarness
          operations={[
            {
              type: 'invalidate',
              targets: [
                { name: 'GetPipelineRun', serviceOptions: { name: 'run-1', namespace: 'ns' } },
              ],
            },
            { type: 'toast', message: 'Pipeline updated' },
          ]}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getRouterWrapper(),
          testContext.wrapper,
          getSnackbarProviderWrapper(),
          getIconProviderWrapper(),
        ])
      );

      const spy = vi.spyOn(testContext.handles.queryClient, 'invalidateQueries');

      await user.click(screen.getByRole('button', { name: 'Run success operations' }));

      expect(spy).toHaveBeenCalledWith({
        queryKey: ['GetPipelineRun', { name: 'run-1', namespace: 'ns' }],
      });
      expect(await screen.findByText('Pipeline updated')).toBeInTheDocument();
    });
  });

  describe('toast', () => {
    it('renders a toast message with the default icon', async () => {
      const user = userEvent.setup();

      render(
        <UseSuccessOperationsTestHarness
          operations={[{ type: 'toast', message: 'Pipeline created' }]}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getRouterWrapper(),
          createServiceProviderTestContext({ request: vi.fn() }).wrapper,
          getSnackbarProviderWrapper(),
          getIconProviderWrapper({
            icons: {
              checkCircle: () => <div>CheckCircle</div>,
            },
          }),
        ])
      );

      await user.click(screen.getByRole('button', { name: 'Run success operations' }));

      expect(await screen.findByText('Pipeline created')).toBeInTheDocument();
      expect(screen.getByText('CheckCircle')).toBeInTheDocument();
    });

    it('renders a toast with a custom icon', async () => {
      const user = userEvent.setup();

      render(
        <UseSuccessOperationsTestHarness
          operations={[{ type: 'toast', message: 'Pipeline created', icon: 'rocket' }]}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getRouterWrapper(),
          createServiceProviderTestContext({ request: vi.fn() }).wrapper,
          getSnackbarProviderWrapper(),
          getIconProviderWrapper({
            icons: {
              rocket: () => <div>Rocket</div>,
            },
          }),
        ])
      );

      await user.click(screen.getByRole('button', { name: 'Run success operations' }));

      expect(await screen.findByText('Pipeline created')).toBeInTheDocument();
      expect(screen.getByText('Rocket')).toBeInTheDocument();
    });

    it('navigates when the toast action has a route', async () => {
      const user = userEvent.setup();

      render(
        <UseSuccessOperationsTestHarness
          operations={[
            {
              type: 'toast',
              message: 'Pipeline created',
              action: { label: 'View', route: '/pipelines/run-1' },
            },
          ]}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getRouterWrapper({ location: '/start' }),
          createServiceProviderTestContext({ request: vi.fn() }).wrapper,
          getSnackbarProviderWrapper(),
          getIconProviderWrapper(),
        ])
      );

      await user.click(screen.getByRole('button', { name: 'Run success operations' }));
      await screen.findByText('Pipeline created');
      await user.click(screen.getAllByRole('button', { name: 'View' })[0]);

      expect(screen.getByText(/Current pathname: \/pipelines\/run-1/)).toBeInTheDocument();
    });

    it('dismisses the toast when the action has no route', async () => {
      const user = userEvent.setup();

      render(
        <UseSuccessOperationsTestHarness
          operations={[
            {
              type: 'toast',
              message: 'Pipeline created',
              action: { label: 'Dismiss' },
            },
          ]}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getRouterWrapper({ location: '/start' }),
          createServiceProviderTestContext({ request: vi.fn() }).wrapper,
          getSnackbarProviderWrapper(),
          getIconProviderWrapper(),
        ])
      );

      await user.click(screen.getByRole('button', { name: 'Run success operations' }));
      await screen.findByText('Pipeline created');
      await user.click(screen.getAllByRole('button', { name: 'Dismiss' })[0]);

      await waitFor(() => {
        expect(screen.queryByText('Pipeline created')).not.toBeInTheDocument();
      });
      expect(screen.getByText(/Current pathname: \/start/)).toBeInTheDocument();
    });

    it('interpolates the toast message from the response payload', async () => {
      const user = userEvent.setup();

      render(
        <UseSuccessOperationsTestHarness
          operations={[
            { type: 'toast', message: interpolate('Pipeline ${response.name} created') },
          ]}
          response={{ name: 'training-v2' }}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getRouterWrapper(),
          createServiceProviderTestContext({ request: vi.fn() }).wrapper,
          getSnackbarProviderWrapper(),
          getIconProviderWrapper(),
        ])
      );

      await user.click(screen.getByRole('button', { name: 'Run success operations' }));

      expect(await screen.findByText('Pipeline training-v2 created')).toBeInTheDocument();
    });
  });
});
