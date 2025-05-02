import { renderHook, waitFor } from '@testing-library/react';
import { vi } from 'vitest';

import { useStudioQuery } from '#core/hooks/use-studio-query';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { getServiceProviderWrapper } from '#core/test/wrappers/get-service-provider-wrapper';

import type { QueryOptions } from '#core/types/query-types';

describe('useStudioQuery', () => {
  const mockRequest = vi.fn().mockResolvedValue(null);

  beforeEach(() => {
    mockRequest.mockClear();
  });

  describe('when query returns no data', () => {
    test('returns data as is', async () => {
      mockRequest.mockResolvedValue(null);

      const { result } = renderHook(
        () => useStudioQuery({ queryName: 'ListAnything', serviceOptions: {} }),
        buildWrapper([getRouterWrapper(), getServiceProviderWrapper({ request: mockRequest })])
      );

      await waitFor(() => {
        expect(result.current.data).toBe(null);
      });
    });

    test('returns other properties as-is', async () => {
      mockRequest.mockResolvedValue(null);

      const { result } = renderHook(
        () => useStudioQuery({ queryName: 'ListAnything', serviceOptions: {} }),
        buildWrapper([getRouterWrapper(), getServiceProviderWrapper({ request: mockRequest })])
      );

      await waitFor(() => {
        expect(result.current.error).toBe(null);
        expect(result.current.isLoading).toBe(false);
      });
    });
  });

  describe('when query returns data', () => {
    test('returns data as is', async () => {
      mockRequest.mockResolvedValue({ test: 'data' });

      const { result } = renderHook(
        () =>
          useStudioQuery({
            queryName: 'ListAnything',
            serviceOptions: {},
          }),
        buildWrapper([getRouterWrapper(), getServiceProviderWrapper({ request: mockRequest })])
      );

      await waitFor(() => {
        expect(result.current.data).toEqual({ test: 'data' });
      });
    });

    test('returns other properties as-is', async () => {
      mockRequest.mockResolvedValue({ test: 'data' });

      const { result } = renderHook(
        () =>
          useStudioQuery({
            queryName: 'ListAnything',
            serviceOptions: {},
          }),
        buildWrapper([getRouterWrapper(), getServiceProviderWrapper({ request: mockRequest })])
      );

      await waitFor(() => {
        expect(result.current.error).toBe(null);
        expect(result.current.isLoading).toBe(false);
      });
    });
  });

  describe('query options passed to useStudioQuery', () => {
    beforeEach(() => {
      mockRequest.mockResolvedValue({});
    });

    test('defaults namespace to projectId when omitted from serviceOptions args', async () => {
      renderHook(
        () => useStudioQuery({ queryName: 'GetDataset', serviceOptions: {} }),
        buildWrapper([
          getRouterWrapper({ location: '/ma-dev-test' }),
          getServiceProviderWrapper({ request: mockRequest }),
        ])
      );

      await waitFor(() => {
        expect(mockRequest).toHaveBeenCalledWith(
          'GetDataset',
          expect.objectContaining({ namespace: 'ma-dev-test' })
        );
      });
    });

    test('prefers provided namespace', async () => {
      renderHook(
        () =>
          useStudioQuery({
            queryName: 'GetDataset',
            serviceOptions: { namespace: 'provided-namespace' },
          }),
        buildWrapper([
          getRouterWrapper({ location: '/ma-dev-test' }),
          getServiceProviderWrapper({ request: mockRequest }),
        ])
      );

      await waitFor(() => {
        expect(mockRequest).toHaveBeenCalledWith(
          'GetDataset',
          expect.objectContaining({ namespace: 'provided-namespace' })
        );
      });
    });

    test('passes clientOptions to useQuery', async () => {
      const clientOptions: QueryOptions = {
        enabled: false,
      };

      renderHook(
        () =>
          useStudioQuery({
            queryName: 'GetDataset',
            serviceOptions: {},
            clientOptions,
          }),
        buildWrapper([
          getRouterWrapper({ location: '/ma-dev-test' }),
          getServiceProviderWrapper({ request: mockRequest }),
        ])
      );

      await waitFor(() => {
        expect(mockRequest).not.toHaveBeenCalled();
      });
    });
  });

  describe('queryFn implementation', () => {
    beforeEach(() => {
      mockRequest.mockResolvedValue({ test: 'data' });
    });

    test('calls request with correct arguments', async () => {
      renderHook(
        () =>
          useStudioQuery({
            queryName: 'GetDataset',
            serviceOptions: { filter: 'active' },
          }),
        buildWrapper([
          getRouterWrapper({ location: '/ma-dev-test' }),
          getServiceProviderWrapper({ request: mockRequest }),
        ])
      );

      await waitFor(() => {
        expect(mockRequest).toHaveBeenCalledWith('GetDataset', {
          filter: 'active',
          namespace: 'ma-dev-test',
        });
      });
    });
  });
});
