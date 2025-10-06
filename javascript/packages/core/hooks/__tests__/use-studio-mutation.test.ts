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

    result.current.mutate({ pipelineRun: { name: 'test-run' } });

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith('CreatePipelineRun', {
        pipelineRun: { name: 'test-run' },
      });
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

    result.current.mutate({ pipelineRun: { name: 'test-run' } });

    await waitFor(() => {
      expect(result.current.data).toEqual(mockResponse);
      expect(result.current.isSuccess).toBe(true);
    });
  });

  test('passes onSuccess callback with correct parameters', async () => {
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

    const testVariables = { pipelineRun: { name: 'test-run' } };
    result.current.mutate(testVariables);

    await waitFor(() => {
      expect(onSuccess).toHaveBeenCalledWith(mockResponse, testVariables, undefined);
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

    const testVariables = { pipelineRun: { name: 'test-run' } };
    result.current.mutate(testVariables);

    await waitFor(() => {
      expect(onError).toHaveBeenCalledWith(expect.any(ApplicationError), testVariables, undefined);
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

    result.current.mutate({ pipelineRun: { name: 'test-run' } });

    await waitFor(() => {
      expect(result.current.error).not.toBeNull();
    });

    const error = result.current.error!;
    expect(error.message).toEqual('Pipeline run creation failed');
    expect(error.source).toEqual('mutation-normalizer');
  });
});
