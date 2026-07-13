import { renderHook, screen, waitFor } from '@testing-library/react';
import { vi } from 'vitest';

import { GrpcStatusCode } from '#core/constants/grpc-status-codes';
import { useStudioMutation } from '#core/hooks/use-studio-mutation/use-studio-mutation';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import {
  createQueryMockRouter,
  getServiceProviderWrapper,
} from '#core/test/wrappers/get-service-provider-wrapper';
import { getSnackbarProviderWrapper } from '#core/test/wrappers/get-snackbar-provider-wrapper';
import { getUserProviderWrapper } from '#core/test/wrappers/get-user-provider-wrapper';
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
        getSnackbarProviderWrapper(),
      ])
    );

    result.current.mutate({ name: 'test-run' });

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith('CreatePipelineRun', { name: 'test-run' }, {});
    });
  });

  test('applies config.middleware to variables before calling request', async () => {
    const mockResponse = { id: 'test-id' };
    const mockRequest = createQueryMockRouter({
      CreatePipelineRun: mockResponse,
    });

    const { result } = renderHook(
      () =>
        useStudioMutation({
          mutationName: 'CreatePipelineRun',
          middleware: { operations: [{ destination: 'spec.action', default: 'KILL' }] },
        }),
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper(),
        getServiceProviderWrapper({ request: mockRequest }),
        getSnackbarProviderWrapper(),
      ])
    );

    result.current.mutate({ name: 'test-run' });

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        'CreatePipelineRun',
        expect.objectContaining({ spec: { action: 'KILL' } }),
        {}
      );
    });
  });

  test('reads middleware source from sourceFromObject when provided', async () => {
    const mockResponse = { id: 'test-id' };
    const mockRequest = createQueryMockRouter({
      CreatePipelineRun: mockResponse,
    });

    const { result } = renderHook(
      () =>
        useStudioMutation({
          mutationName: 'CreatePipelineRun',
          middleware: {
            operations: [
              { source: 'name', destination: 'metadata.name', transformation: (v) => v },
            ],
          },
        }),
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper(),
        getServiceProviderWrapper({ request: mockRequest }),
        getSnackbarProviderWrapper(),
      ])
    );

    result.current.mutate({ metadata: {} }, { sourceFromObject: { name: 'from-source' } });

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        'CreatePipelineRun',
        expect.objectContaining({ metadata: { name: 'from-source' } }),
        {}
      );
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
        getSnackbarProviderWrapper(),
      ])
    );

    result.current.mutate({ name: 'test-run' });

    await waitFor(() => {
      expect(result.current.data).toEqual(mockResponse);
      expect(result.current.isPending).toBe(false);
    });
  });

  test('runs successOperations by default on success', async () => {
    const mockResponse = { id: 'test-id' };
    const mockRequest = createQueryMockRouter({
      CreatePipelineRun: mockResponse,
    });

    renderHook(
      () =>
        useStudioMutation({
          mutationName: 'CreatePipelineRun',
          successOperations: [{ type: 'toast', message: 'Pipeline run created' }],
        }),
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper(),
        getServiceProviderWrapper({ request: mockRequest }),
        getSnackbarProviderWrapper(),
      ])
    ).result.current.mutate({ name: 'test-run' });

    expect(await screen.findByText('Pipeline run created')).toBeInTheDocument();
  });

  test('clientOptions.onSuccess runs alongside default successOperations', async () => {
    const mockResponse = { id: 'test-id' };
    const onSuccess = vi.fn();
    const mockRequest = createQueryMockRouter({
      CreatePipelineRun: mockResponse,
    });

    renderHook(
      () =>
        useStudioMutation({
          mutationName: 'CreatePipelineRun',
          successOperations: [{ type: 'toast', message: 'Pipeline run created' }],
          clientOptions: { onSuccess },
        }),
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper(),
        getServiceProviderWrapper({ request: mockRequest }),
        getSnackbarProviderWrapper(),
      ])
    ).result.current.mutate({ name: 'test-run' });

    expect(await screen.findByText('Pipeline run created')).toBeInTheDocument();
    expect(onSuccess).toHaveBeenCalledWith(mockResponse);
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
        getSnackbarProviderWrapper(),
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
        getSnackbarProviderWrapper(),
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
        getSnackbarProviderWrapper(),
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

  test('passes user identity as request headers when user context is provided', async () => {
    const mockRequest = createQueryMockRouter({
      CreatePipelineRun: { id: 'test-id' },
    });

    const { result } = renderHook(
      () => useStudioMutation({ mutationName: 'CreatePipelineRun' }),
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper(),
        getServiceProviderWrapper({ request: mockRequest }),
        getSnackbarProviderWrapper(),
        getUserProviderWrapper({ name: 'Jane Doe', email: 'jane@example.com' }),
      ])
    );

    result.current.mutate({ name: 'test-run' });

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith('CreatePipelineRun', expect.any(Object), {
        'x-user-name': 'Jane Doe',
        'x-user-email': 'jane@example.com',
      });
    });
  });
});
