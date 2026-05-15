import { useLocation } from 'react-router-dom-v5-compat';
import { useQueryClient } from '@tanstack/react-query';
import { renderHook } from '@testing-library/react';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { useSuccessOperations } from '#core/components/actions/use-success-operations';
import { interpolate } from '#core/interpolation/interpolate';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { getServiceProviderWrapper } from '#core/test/wrappers/get-service-provider-wrapper';
import { getSnackbarProviderWrapper } from '#core/test/wrappers/get-snackbar-provider-wrapper';

import type { SuccessOperation } from '#core/components/actions/types';

describe('useSuccessOperations', () => {
  it('does nothing when operations is undefined or empty', () => {
    const { result } = renderHook(
      () => ({
        run: useSuccessOperations(),
        queryClient: useQueryClient(),
      }),
      buildWrapper([
        getBaseProviderWrapper(),
        getRouterWrapper(),
        getServiceProviderWrapper({ request: vi.fn() }),
        getSnackbarProviderWrapper(),
        getIconProviderWrapper(),
      ])
    );
    const spy = vi.spyOn(result.current.queryClient, 'invalidateQueries');
    result.current.run({});
    expect(spy).not.toHaveBeenCalled();
  });

  describe('invalidate', () => {
    it('invalidates a query by name only', () => {
      const operations: SuccessOperation[] = [{ type: 'invalidate', targets: ['ListPipelineRun'] }];
      const { result } = renderHook(
        () => ({
          run: useSuccessOperations(operations),
          queryClient: useQueryClient(),
        }),
        buildWrapper([
          getBaseProviderWrapper(),
          getRouterWrapper(),
          getServiceProviderWrapper({ request: vi.fn() }),
          getSnackbarProviderWrapper(),
          getIconProviderWrapper(),
        ])
      );
      const spy = vi.spyOn(result.current.queryClient, 'invalidateQueries');
      result.current.run({});
      expect(spy).toHaveBeenCalledWith({ queryKey: ['ListPipelineRun'] });
    });

    it('invalidates a query by name + serviceOptions', () => {
      const operations: SuccessOperation[] = [
        {
          type: 'invalidate',
          targets: [{ name: 'GetPipelineRun', serviceOptions: { name: 'run-1', namespace: 'ns' } }],
        },
      ];
      const { result } = renderHook(
        () => ({
          run: useSuccessOperations(operations),
          queryClient: useQueryClient(),
        }),
        buildWrapper([
          getBaseProviderWrapper(),
          getRouterWrapper(),
          getServiceProviderWrapper({ request: vi.fn() }),
          getSnackbarProviderWrapper(),
          getIconProviderWrapper(),
        ])
      );
      const spy = vi.spyOn(result.current.queryClient, 'invalidateQueries');
      result.current.run({});
      expect(spy).toHaveBeenCalledWith({
        queryKey: ['GetPipelineRun', { name: 'run-1', namespace: 'ns' }],
      });
    });

    it('processes multiple targets in order', () => {
      const operations: SuccessOperation[] = [
        { type: 'invalidate', targets: ['ListPipelineRun', 'GetPipelineRun'] },
      ];
      const { result } = renderHook(
        () => ({
          run: useSuccessOperations(operations),
          queryClient: useQueryClient(),
        }),
        buildWrapper([
          getBaseProviderWrapper(),
          getRouterWrapper(),
          getServiceProviderWrapper({ request: vi.fn() }),
          getSnackbarProviderWrapper(),
          getIconProviderWrapper(),
        ])
      );
      const spy = vi.spyOn(result.current.queryClient, 'invalidateQueries');
      result.current.run({});
      expect(spy).toHaveBeenNthCalledWith(1, { queryKey: ['ListPipelineRun'] });
      expect(spy).toHaveBeenNthCalledWith(2, { queryKey: ['GetPipelineRun'] });
    });
  });

  describe('toast', () => {
    function ShowLocation() {
      const { pathname } = useLocation();
      return <div data-testid="location">{pathname}</div>;
    }

    function TestHarness({ operations }: { operations: SuccessOperation[] }) {
      const run = useSuccessOperations(operations);
      return (
        <>
          <button onClick={() => run({})}>fire</button>
          <ShowLocation />
        </>
      );
    }

    it('enqueues a snackbar visible in the DOM', async () => {
      const user = userEvent.setup();
      render(
        <TestHarness operations={[{ type: 'toast', message: 'Created!' }]} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getRouterWrapper(),
          getServiceProviderWrapper({ request: vi.fn() }),
          getSnackbarProviderWrapper(),
          getIconProviderWrapper(),
        ])
      );
      await user.click(screen.getByRole('button', { name: 'fire' }));
      expect(await screen.findByText('Created!')).toBeInTheDocument();
    });

    it('toast action with route navigates on click', async () => {
      const user = userEvent.setup();
      render(
        <TestHarness
          operations={[
            {
              type: 'toast',
              message: 'Created!',
              action: { label: 'View', route: '/projects/p1/runs/run-1' },
            },
          ]}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getRouterWrapper({ location: '/start' }),
          getServiceProviderWrapper({ request: vi.fn() }),
          getSnackbarProviderWrapper(),
          getIconProviderWrapper(),
        ])
      );
      await user.click(screen.getByRole('button', { name: 'fire' }));
      await screen.findByText('Created!');
      // baseui renders the snackbar twice during transitions; either button works.
      await user.click(screen.getAllByRole('button', { name: 'View' })[0]);
      expect(screen.getByTestId('location')).toHaveTextContent('/projects/p1/runs/run-1');
    });

    it('toast action without route dismisses on click', async () => {
      const user = userEvent.setup();
      render(
        <TestHarness operations={[{ type: 'toast', message: 'Done', action: { label: 'OK' } }]} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getRouterWrapper({ location: '/start' }),
          getServiceProviderWrapper({ request: vi.fn() }),
          getSnackbarProviderWrapper(),
          getIconProviderWrapper(),
        ])
      );
      await user.click(screen.getByRole('button', { name: 'fire' }));
      await screen.findByText('Done');
      await user.click(screen.getAllByRole('button', { name: 'OK' })[0]);
      await waitFor(() => {
        expect(screen.queryByText('Done')).not.toBeInTheDocument();
      });
      expect(screen.getByTestId('location')).toHaveTextContent('/start');
    });
  });

  describe('response interpolation', () => {
    it('resolves ${response.X} in toast message', async () => {
      const user = userEvent.setup();
      function TestHarness({ response }: { response: unknown }) {
        const run = useSuccessOperations([
          { type: 'toast', message: interpolate('Pipeline ${response.name} created') },
        ]);
        return <button onClick={() => run(response)}>fire</button>;
      }

      render(
        <TestHarness response={{ name: 'training-v2' }} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getRouterWrapper(),
          getServiceProviderWrapper({ request: vi.fn() }),
          getSnackbarProviderWrapper(),
          getIconProviderWrapper(),
        ])
      );
      await user.click(screen.getByRole('button', { name: 'fire' }));
      expect(await screen.findByText('Pipeline training-v2 created')).toBeInTheDocument();
    });
  });
});
