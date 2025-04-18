import { renderHook } from '@testing-library/react';

import useURLQueryString from '@/hooks/routing/use-url-query-string';
import { buildWrapper } from '@/test/wrappers/build-wrapper';
import { getRouterWrapper } from '@/test/wrappers/get-router-wrapper';

describe('useURLQueryString', () => {
  describe('with no query parameters', () => {
    it('returns empty object', () => {
      const { result } = renderHook(
        () => useURLQueryString(),
        buildWrapper([getRouterWrapper({ location: '/ma/ma-customer-sandbox/train/pipelines' })])
      );

      expect(result.current).toEqual({});
    });
  });

  describe('with single query parameter', () => {
    it('parses single parameter correctly', () => {
      const { result } = renderHook(
        () => useURLQueryString<{ revisionId: string }>(),
        buildWrapper([
          getRouterWrapper({
            location: '/ma/ma-customer-sandbox/train/pipelines?revisionId=123',
          }),
        ])
      );

      expect(result.current).toEqual({ revisionId: '123' });
    });
  });

  describe('with multiple query parameters', () => {
    it('parses multiple parameters correctly', () => {
      const { result } = renderHook(
        () => useURLQueryString<{ revisionId: string; status: string }>(),
        buildWrapper([
          getRouterWrapper({
            location: '/ma/ma-customer-sandbox/train/pipelines?revisionId=123&status=active',
          }),
        ])
      );

      expect(result.current).toEqual({
        revisionId: '123',
        status: 'active',
      });
    });
  });

  describe('with encoded query parameters', () => {
    it('decodes parameters correctly', () => {
      const { result } = renderHook(
        () =>
          useURLQueryString<{
            filter: string;
            search: string;
          }>(),
        buildWrapper([
          getRouterWrapper({
            location: '/ma/project?filter=status%3Dactive&search=test%20query',
          }),
        ])
      );

      expect(result.current).toEqual({
        filter: 'status=active',
        search: 'test query',
      });
    });
  });

  describe('with duplicate query parameters', () => {
    it('uses last value for duplicate parameters', () => {
      const { result } = renderHook(
        () =>
          useURLQueryString<{
            tag: string;
          }>(),
        buildWrapper([
          getRouterWrapper({
            location: '/ma/project?tag=v1&tag=v2',
          }),
        ])
      );

      expect(result.current).toEqual({
        tag: 'v2',
      });
    });
  });

  describe('with empty query parameter value', () => {
    it('handles empty values correctly', () => {
      const { result } = renderHook(
        () =>
          useURLQueryString<{
            empty: string;
            normal: string;
          }>(),
        buildWrapper([
          getRouterWrapper({
            location: '/ma/project?empty=&normal=value',
          }),
        ])
      );

      expect(result.current).toEqual({
        empty: '',
        normal: 'value',
      });
    });
  });
});
