import { useQueryClient } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import { vi } from 'vitest';

import { GrpcStatusCode } from '#core/constants/grpc-status-codes';
import { useStudioMutation } from '#core/hooks/use-studio-mutation';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import {
  createQueryMockRouter,
  getServiceProviderWrapper,
} from '#core/test/wrappers/get-service-provider-wrapper';
import { ApplicationError } from '#core/types/error-types';

import type { ErrorNormalizer } from '#core/types/error-types';

describe('useStudioMutation', () => {
  test('calls request with correct mutation name and variables', async () => {
    const mockResponse = { id: 'test-id', name: 'test-pipeline-run' };
    const mockRequest = createQueryMockRouter({
      CreatePipelineRun: mockResponse,
    });

    const { result } = renderHook(
      () => useStudioMutation({ mutationName: 'CreatePipelineRun' }),
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    result.current.mutate({ name: 'test-run' });

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith('CreatePipelineRun', { name: 'test-run' });
    });
  });

  test('returns mutation response data', async () => {
    const mockResponse = { id: 'test-id', name: 'test-pipeline-run' };
    const mockRequest = createQueryMockRouter({
      CreatePipelineRun: mockResponse,
    });

    const { result } = renderHook(
      () => useStudioMutation({ mutationName: 'CreatePipelineRun' }),
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    result.current.mutate({ name: 'test-run' });

    await waitFor(() => {
      expect(result.current.data).toEqual(mockResponse);
      expect(result.current.isSuccess).toBe(true);
    });
  });

  test('passes onSuccess callback with response data', async () => {
    const mockResponse = { id: 'test-id' };
    const onSuccess = vi.fn();
    const mockRequest = createQueryMockRouter({
      CreatePipelineRun: mockResponse,
    });

    const { result } = renderHook(
      () =>
        useStudioMutation({
          mutationName: 'CreatePipelineRun',
          clientOptions: { onSuccess },
        }),
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper(),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    result.current.mutate({ name: 'test-run' });

    await waitFor(() => {
      expect(onSuccess).toHaveBeenCalledWith(mockResponse);
    });
  });

  test('passes onError callback with normalized ApplicationError', async () => {
    const testError = new Error('Request failed');
    const onError = vi.fn();
    const mockRequest = createQueryMockRouter({
      CreatePipelineRun: testError,
    });

    const { result } = renderHook(
      () =>
        useStudioMutation({
          mutationName: 'CreatePipelineRun',
          clientOptions: { onError },
        }),
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper(),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    result.current.mutate({ name: 'test-run' });

    await waitFor(() => {
      expect(onError).toHaveBeenCalledWith(expect.any(ApplicationError));
    });
  });

  test('normalizes errors', async () => {
    const customError = new Error('Pipeline run creation failed');
    (customError as unknown as Record<string, unknown>).code = 400;
    (customError as unknown as Record<string, unknown>).meta = { requestId: 'req-456' };
    (customError as unknown as Record<string, unknown>).isRpcError = true;

    const customNormalizer: ErrorNormalizer = (error: unknown) => {
      if (error instanceof Error && 'isRpcError' in error) {
        const rpcError = error as Error & Record<string, unknown>;
        return new ApplicationError(rpcError.message, GrpcStatusCode.INVALID_ARGUMENT, {
          source: 'mutation-normalizer',
          meta: rpcError.meta as Record<string, unknown>,
        });
      }
      return null;
    };

    const mockRequest = createQueryMockRouter({
      CreatePipelineRun: customError,
    });

    const { result } = renderHook(
      () => useStudioMutation({ mutationName: 'CreatePipelineRun' }),
      buildWrapper([
        getErrorProviderWrapper({ normalizeError: customNormalizer }),
        getRouterWrapper(),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    result.current.mutate({ name: 'test-run' });

    await waitFor(() => {
      expect(result.current.error).not.toBeNull();
    });

    const error = result.current.error!;
    expect(error.message).toEqual('Pipeline run creation failed');
    expect(error.source).toEqual('mutation-normalizer');
  });

  describe('auto-invalidation by mutationName', () => {
    test('Create{Entity} invalidates Get{Entity} and List{Entity} on settle', async () => {
      const mockRequest = createQueryMockRouter({ CreatePipelineRun: {} });
      const { result } = renderHook(
        () => ({
          mutation: useStudioMutation({ mutationName: 'CreatePipelineRun' }),
          queryClient: useQueryClient(),
        }),
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper(),
          getServiceProviderWrapper({ request: mockRequest }),
        ])
      );
      const spy = vi.spyOn(result.current.queryClient, 'invalidateQueries');

      result.current.mutation.mutate({ name: 'run-1' });
      await waitFor(() => expect(result.current.mutation.isSuccess).toBe(true));

      expect(spy).toHaveBeenCalledWith({ queryKey: ['GetPipelineRun'] });
      expect(spy).toHaveBeenCalledWith({ queryKey: ['ListPipelineRun'] });
    });

    test('Update{Entity} invalidates on settle', async () => {
      const mockRequest = createQueryMockRouter({ UpdateTriggerRun: {} });
      const { result } = renderHook(
        () => ({
          mutation: useStudioMutation({ mutationName: 'UpdateTriggerRun' }),
          queryClient: useQueryClient(),
        }),
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper(),
          getServiceProviderWrapper({ request: mockRequest }),
        ])
      );
      const spy = vi.spyOn(result.current.queryClient, 'invalidateQueries');

      result.current.mutation.mutate({ name: 'trigger-1' });
      await waitFor(() => expect(result.current.mutation.isSuccess).toBe(true));

      expect(spy).toHaveBeenCalledWith({ queryKey: ['GetTriggerRun'] });
      expect(spy).toHaveBeenCalledWith({ queryKey: ['ListTriggerRun'] });
    });

    test('DeleteCollection{Entity} parses correctly (matched before Delete)', async () => {
      const mockRequest = createQueryMockRouter({ DeleteCollectionPipelineRun: {} });
      const { result } = renderHook(
        () => ({
          mutation: useStudioMutation({ mutationName: 'DeleteCollectionPipelineRun' }),
          queryClient: useQueryClient(),
        }),
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper(),
          getServiceProviderWrapper({ request: mockRequest }),
        ])
      );
      const spy = vi.spyOn(result.current.queryClient, 'invalidateQueries');

      result.current.mutation.mutate({});
      await waitFor(() => expect(result.current.mutation.isSuccess).toBe(true));

      expect(spy).toHaveBeenCalledWith({ queryKey: ['GetPipelineRun'] });
      expect(spy).toHaveBeenCalledWith({ queryKey: ['ListPipelineRun'] });
    });

    test('mutationName not matching the verb pattern skips auto-invalidation', async () => {
      const mockRequest = createQueryMockRouter({ DoSomethingCustom: {} });
      const { result } = renderHook(
        () => ({
          mutation: useStudioMutation({ mutationName: 'DoSomethingCustom' }),
          queryClient: useQueryClient(),
        }),
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper(),
          getServiceProviderWrapper({ request: mockRequest }),
        ])
      );
      const spy = vi.spyOn(result.current.queryClient, 'invalidateQueries');

      result.current.mutation.mutate({});
      await waitFor(() => expect(result.current.mutation.isSuccess).toBe(true));

      expect(spy).not.toHaveBeenCalled();
    });

    test('invalidates on failure as well as success', async () => {
      const mockRequest = createQueryMockRouter({
        UpdatePipelineRun: new Error('Failed'),
      });
      const { result } = renderHook(
        () => ({
          mutation: useStudioMutation({ mutationName: 'UpdatePipelineRun' }),
          queryClient: useQueryClient(),
        }),
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper(),
          getServiceProviderWrapper({ request: mockRequest }),
        ])
      );
      const spy = vi.spyOn(result.current.queryClient, 'invalidateQueries');

      result.current.mutation.mutate({});
      await waitFor(() => expect(result.current.mutation.isError).toBe(true));

      expect(spy).toHaveBeenCalledWith({ queryKey: ['GetPipelineRun'] });
      expect(spy).toHaveBeenCalledWith({ queryKey: ['ListPipelineRun'] });
    });
  });
});
