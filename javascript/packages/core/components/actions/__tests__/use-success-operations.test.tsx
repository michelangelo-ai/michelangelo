import { useQueryClient } from '@tanstack/react-query';
import { renderHook } from '@testing-library/react';

import { useSuccessOperations } from '#core/components/actions/use-success-operations';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { getServiceProviderWrapper } from '#core/test/wrappers/get-service-provider-wrapper';
import { getSnackbarProviderWrapper } from '#core/test/wrappers/get-snackbar-provider-wrapper';

import type { SuccessOperation } from '#core/components/actions/types';

describe('useSuccessOperations — invalidate', () => {
  it('invalidates a query by name only', () => {
    const operations: SuccessOperation[] = [{ type: 'invalidate', targets: ['ListPipelineRun'] }];
    const { result } = renderHook(
      () => ({ run: useSuccessOperations(operations), queryClient: useQueryClient() }),
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
      () => ({ run: useSuccessOperations(operations), queryClient: useQueryClient() }),
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
      () => ({ run: useSuccessOperations(operations), queryClient: useQueryClient() }),
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
