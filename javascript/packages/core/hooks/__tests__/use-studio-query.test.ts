import { renderHook } from '@testing-library/react';
import { vi } from 'vitest';

import { useStudioQuery } from '#core/hooks/use-studio-query';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { getServiceProviderWrapper } from '#core/test/wrappers/get-service-provider-wrapper';

describe('useStudioQuery', () => {
  const mockUseQuery = vi.fn().mockReturnValue({ data: null, error: null, isLoading: false });

  beforeEach(() => {
    mockUseQuery.mockClear();
  });

  describe('when query returns no data', () => {
    let result: ReturnType<typeof useStudioQuery>;

    beforeAll(() => {
      mockUseQuery.mockReturnValue({ data: null, error: null, isLoading: false });

      const { result: _result } = renderHook(
        () => useStudioQuery({ queryName: 'ListAnything', serviceOptions: {} }),
        buildWrapper([getRouterWrapper(), getServiceProviderWrapper({ useQuery: mockUseQuery })])
      );

      result = _result.current;
    });

    test('returns data as is', () => {
      expect(result.data).toBe(null);
    });

    test('returns other properties as-is', () => {
      expect(result.error).toBe(null);
      expect(result.isLoading).toBe(false);
    });
  });

  describe('when clientOptions is not provided', () => {
    let result: ReturnType<typeof useStudioQuery>;

    beforeAll(() => {
      mockUseQuery.mockReturnValue({ data: { test: 'data' }, error: null, isLoading: false });

      const { result: _result } = renderHook(
        () =>
          useStudioQuery({
            queryName: 'ListAnything',
            serviceOptions: {},
          }),
        buildWrapper([getRouterWrapper(), getServiceProviderWrapper({ useQuery: mockUseQuery })])
      );

      result = _result.current;
    });

    test('returns data as is', () => {
      expect(result.data).toEqual({ test: 'data' });
    });

    test('returns other properties as-is', () => {
      expect(result.error).toBe(null);
      expect(result.isLoading).toBe(false);
    });
  });

  describe('query options passed to useRpcQuery', () => {
    beforeEach(() => {
      mockUseQuery.mockReturnValue({ data: {}, isLoading: false });
    });

    test('defaults namespace to projectId when omitted from serviceOptions args', () => {
      renderHook(
        () => useStudioQuery({ queryName: 'GetDataset', serviceOptions: {} }),
        buildWrapper([
          getRouterWrapper({ location: '/ma-dev-test' }),
          getServiceProviderWrapper({ useQuery: mockUseQuery }),
        ])
      );

      expect(mockUseQuery).toHaveBeenCalledWith(
        'GetDataset',
        expect.objectContaining({ namespace: 'ma-dev-test' }),
        undefined
      );
    });

    test('prefers provided namespace', () => {
      renderHook(
        () =>
          useStudioQuery({
            queryName: 'GetDataset',
            serviceOptions: { namespace: 'provided-namespace' },
          }),
        buildWrapper([
          getRouterWrapper({ location: '/ma-dev-test' }),
          getServiceProviderWrapper({ useQuery: mockUseQuery }),
        ])
      );

      expect(mockUseQuery).toHaveBeenCalledWith(
        'GetDataset',
        expect.objectContaining({ namespace: 'provided-namespace' }),
        undefined
      );
    });
  });
});
