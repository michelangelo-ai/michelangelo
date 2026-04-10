import { renderHook } from '@testing-library/react';
import { lowerCase, upperCase } from 'lodash';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getInterpolationProviderWrapper } from '#core/test/wrappers/get-interpolation-provider-wrapper';
import { getRepeatedLayoutProviderWrapper } from '#core/test/wrappers/get-repeated-layout-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { createMockProject, createMockUser } from '../__fixtures__/mock-context';
import { interpolate } from '../interpolate';
import { useInterpolationResolver } from '../use-interpolation-resolver';

import type { ExclusionCheck, InterpolationContext, UserDataSources } from '../types';

describe('useInterpolationResolver', () => {
  let resolve: <T>(
    variables: T,
    input?: Partial<UserDataSources>,
    excludeProperty?: ExclusionCheck
  ) => T;

  beforeEach(() => {
    const { result } = renderHook(
      () => useInterpolationResolver(),
      buildWrapper([
        getInterpolationProviderWrapper({ user: createMockUser(), project: createMockProject() }),
        getRouterWrapper(),
      ])
    );
    resolve = (interpolator, input, excludeProperty) =>
      result.current(
        interpolator,
        {
          page: { metadata: { namespace: 'abc-123', name: 'tester' }, spec: { id: 'SOME_ID' } },
          row: {
            metadata: { namespace: 'row-namespace', name: 'row-name' },
            spec: { id: 'row-spec-id' },
          },
          initialValues: {
            metadata: { namespace: 'abc-123-original', name: 'tester-original' },
            spec: { id: 'SOME_ID-original' },
          },
          ...input,
        },
        excludeProperty
      );
  });

  describe('No interpolation', () => {
    test('Does nothing when no variable exists in the string', () => {
      expect(resolve('test 123')).toBe('test 123');
    });
  });

  describe('String Interpolation', () => {
    test('resolves page metadata interpolation with text before', () => {
      const interpolation = interpolate('test ${page.metadata.namespace}');

      expect(resolve(interpolation)).toBe('test abc-123');
    });

    test('resolves page metadata interpolation with text after', () => {
      const interpolation = interpolate('${page.metadata.namespace} ok');

      expect(resolve(interpolation)).toBe('abc-123 ok');
    });

    test('resolves page metadata interpolation with text before and after', () => {
      const interpolation = interpolate('test ${page.metadata.namespace} ok');

      expect(resolve(interpolation)).toBe('test abc-123 ok');
    });

    test('resolves multiple variables in one string', () => {
      const interpolation = interpolate(
        'test ${page.metadata.namespace} - ${data.metadata.name} ?'
      );

      expect(resolve(interpolation)).toBe('test abc-123 - row-name ?');
    });

    test('resolves mixed data sources (row vs page)', () => {
      const interpolation = interpolate(
        'row: ${row.metadata.namespace}, page: ${page.metadata.name}'
      );

      expect(resolve(interpolation)).toBe('row: row-namespace, page: tester');
    });

    test('resolves initial values interpolation', () => {
      const interpolation = interpolate('test ${initialValues.metadata.namespace}');

      expect(resolve(interpolation)).toBe('test abc-123-original');
    });

    test('handles undefined interpolations gracefully', () => {
      const undefinedInterpolation = interpolate('test ${page.doesnotexist}');

      expect(resolve(undefinedInterpolation)).toBe(undefinedInterpolation);
    });

    test('returns input for later resolutions when interpolation throws', () => {
      const interpolation = interpolate('${page.spec.manifest.content.pusher.model_desc}');

      expect(resolve(interpolation)).toBe(interpolation);
    });
  });

  describe('FunctionInterpolation', () => {
    test('Resolves successfully', () => {
      const interpolation = interpolate(
        ({ page }: InterpolationContext) => `test ${lowerCase(page.spec.id as string)}`
      );

      expect(resolve(interpolation)).toBe('test some id');
    });

    test('If it throws, return the input for later resolutions', () => {
      const interpolation = interpolate(({ page }) =>
        // eslint-disable-next-line --  intentionally passing invalid data
        lowerCase(page.some.prop.that.its.not.available)
      );

      expect(resolve(interpolation)).toBe(interpolation);
    });

    test('Sources page and row data', () => {
      const interpolation = interpolate(
        ({ page, row }: InterpolationContext) =>
          `page name ${page.metadata.name} row name ${row.metadata.name}`
      );

      expect(resolve(interpolation)).toBe('page name tester row name row-name');
    });

    test('Sources initial form values', () => {
      const interpolation = interpolate(
        ({ page, initialValues }: InterpolationContext) =>
          `initial name ${initialValues.metadata.name}, page name ${page.metadata.name}`
      );

      expect(resolve(interpolation)).toBe('initial name tester-original, page name tester');
    });
  });

  describe('Recursive resolution', () => {
    test('resolves interpolations in arrays', () => {
      const input = {
        arr: [
          interpolate('${page.metadata.name}'),
          'some-other-val',
          interpolate('${page.metadata.namespace}'),
        ],
      };

      const result = resolve(input);

      expect(result.arr).toEqual(['tester', 'some-other-val', 'abc-123']);
    });

    test('resolves interpolations in nested objects', () => {
      const input = {
        root: 'root-val',
        nested: {
          prop: interpolate('${page.metadata.name}'),
        },
      };

      const result = resolve(input);

      expect(result).toEqual({
        root: 'root-val',
        nested: {
          prop: 'tester',
        },
      });
    });

    test('preserves object symbols while resolving interpolations', () => {
      const symbol = Symbol('test');
      const input = {
        [symbol]: 'symbol-value',
        normal: interpolate('${page.metadata.name}'),
      };

      const result = resolve(input);

      expect(result.normal).toBe('tester');
      expect(result[symbol]).toBe('symbol-value');
    });

    test('resolves complex nested structure with multiple data types', () => {
      const input = {
        root: 'root-val',
        arr: [
          interpolate('${page.metadata.name}'),
          'some-other-val',
          interpolate('${page.metadata.namespace}'),
        ],
        nested: {
          prop: interpolate(({ page }: InterpolationContext) => {
            return page.metadata.namespace === 'abc-123'
              ? interpolate(({ page }: InterpolationContext) =>
                  upperCase(page.metadata.name as string)
                )
              : 'abc-123';
          }),
        },
      };

      const result = resolve(input);

      expect(result).toEqual({
        root: 'root-val',
        arr: ['tester', 'some-other-val', 'abc-123'],
        nested: {
          prop: 'TESTER',
        },
      });
    });
  });

  describe('Advanced integration scenarios', () => {
    test('resolves function interpolation with data transformation', () => {
      const ownerInterpolation = interpolate(
        ({ data }: InterpolationContext) => `${data.user.username}@${data.user.email.split('@')[1]}`
      );

      const resolved = resolve(ownerInterpolation, {
        row: { user: createMockUser() },
      });

      expect(resolved).toBe('testuser@uber.com');
    });

    test('resolves conditional logic in function interpolations', () => {
      const conditionalInterpolation = interpolate(
        ({ data, initialValues }: InterpolationContext) => {
          return data.priority > 5 ? initialValues.fallback : data.value;
        }
      );

      // Test with low priority (should use data.value)
      const lowPriorityResult = resolve(conditionalInterpolation, {
        row: { priority: 3, value: 'low-priority' },
        initialValues: { fallback: 'fallback-value' },
      });
      expect(lowPriorityResult).toBe('low-priority');

      // Test with high priority (should use fallback)
      const highPriorityResult = resolve(conditionalInterpolation, {
        row: { priority: 8, value: 'high-priority' },
        initialValues: { fallback: 'fallback-value' },
      });
      expect(highPriorityResult).toBe('fallback-value');
    });

    test('resolves complex nested object with multiple interpolation types', () => {
      const complexSchema = {
        metadata: {
          name: interpolate('${page.title}'),
          owner: interpolate(
            ({ data }: InterpolationContext) =>
              `${data.user.username}@${data.user.email.split('@')[1]}`
          ),
        },
        content: [
          {
            title: interpolate('${page.metadata.title}'),
            dynamic: interpolate(({ data, initialValues }: InterpolationContext) => {
              return data.priority > 5 ? initialValues.fallback : data.value;
            }),
          },
        ],
      };

      const resolved = resolve(complexSchema, {
        page: { title: 'Test Project', metadata: { title: 'Test Page' } },
        row: { priority: 3, value: 'low-priority', user: createMockUser() },
        initialValues: { fallback: 'fallback-value' },
      });

      expect(resolved).toEqual({
        metadata: {
          name: 'Test Project',
          owner: 'testuser@uber.com',
        },
        content: [
          {
            title: 'Test Page',
            dynamic: 'low-priority',
          },
        ],
      });
    });

    test('handles circular reference protection', () => {
      const { result } = renderHook(
        () => useInterpolationResolver(),
        buildWrapper([getRouterWrapper()])
      );

      const circularData: Record<string, unknown> = { name: 'test' };
      circularData.self = circularData;

      const schema = {
        title: interpolate('${data.name}'),
        nested: {
          value: interpolate(({ data }: InterpolationContext) => data.name.toUpperCase()),
        },
      };

      const resolved = result.current(schema, { row: circularData });

      expect(resolved).toEqual({
        title: 'test',
        nested: {
          value: 'TEST',
        },
      });
    });
  });

  describe('Symbol preservation', () => {
    test('preserves React symbols and framework metadata', () => {
      const { result } = renderHook(
        () => useInterpolationResolver(),
        buildWrapper([getRouterWrapper()])
      );

      const reactSymbol = Symbol.for('react.element');
      const customSymbol = Symbol('custom');

      const objectWithSymbols = {
        [reactSymbol]: 'react-metadata',
        [customSymbol]: 'custom-value',
        normalProp: interpolate('${data.value}'),
      };

      const resolved = result.current<typeof objectWithSymbols>(objectWithSymbols, {
        row: { value: 'interpolated' },
      });

      expect(resolved.normalProp).toBe('interpolated');
      expect(resolved[reactSymbol]).toBe('react-metadata');
      expect(resolved[customSymbol]).toBe('custom-value');
    });
  });

  describe('Performance and caching behavior', () => {
    test('caches page and initialValues across multiple resolutions', () => {
      const { result } = renderHook(
        () => useInterpolationResolver(),
        buildWrapper([getRouterWrapper()])
      );

      const interpolation = interpolate(
        ({ page, initialValues }: InterpolationContext) =>
          `${page?.title ?? 'no-page'}-${initialValues?.name ?? 'no-initial'}`
      );

      // First resolution with page data
      const firstResult = result.current(interpolation, {
        page: { title: 'First Page' },
      });
      expect(firstResult).toBe('First Page-no-initial');

      // Second resolution with initialValues, should have cached page
      const secondResult = result.current(interpolation, {
        initialValues: { name: 'Initial' },
      });
      expect(secondResult).toBe('First Page-Initial');

      // Third resolution with new page, should update cache
      const thirdResult = result.current(interpolation, {
        page: { title: 'New Page' },
      });
      expect(thirdResult).toBe('New Page-Initial');
    });
  });

  describe('Error handling and resilience', () => {
    test('handles errors gracefully and allows future resolution', () => {
      const { result } = renderHook(
        () => useInterpolationResolver(),
        buildWrapper([getRouterWrapper()])
      );

      const faultyInterpolation = interpolate(
        ({ data }: InterpolationContext) => data.missing.property.access
      );

      // First resolution should fail and return the interpolation object
      const firstResult = result.current(faultyInterpolation, { row: {} });
      expect(firstResult).toBe(faultyInterpolation);

      // Second resolution with proper data should succeed
      const secondResult = result.current(faultyInterpolation, {
        row: { missing: { property: { access: 'success' } } },
      });
      expect(secondResult).toBe('success');
    });
  });

  describe('Provider integration', () => {
    test('resolves user and project data via context extension', () => {
      const userInterpolation = interpolate('${user.username}');
      const projectInterpolation = interpolate('${project.name}');

      expect(resolve(userInterpolation)).toBe('testuser');
      expect(resolve(projectInterpolation)).toBe('Test Project');
    });

    test('uses repeated layout context when provided', () => {
      const { result } = renderHook(
        () => useInterpolationResolver(),
        buildWrapper([
          getInterpolationProviderWrapper({ user: createMockUser(), project: createMockProject() }),
          getRepeatedLayoutProviderWrapper({ index: 2, rootFieldPath: 'items' }),
          getRouterWrapper(),
        ])
      );

      const interpolation = interpolate(
        ({ repeatedLayoutContext }) =>
          `index: ${repeatedLayoutContext?.index}, path: ${repeatedLayoutContext?.rootFieldPath}`
      );

      const resolved = result.current(interpolation, {
        page: { metadata: { namespace: 'abc-123', name: 'tester' }, spec: { id: 'SOME_ID' } },
        row: { metadata: { namespace: 'row-namespace', name: 'row-name' }, spec: { id: 'row-spec-id' } },
        initialValues: { metadata: { namespace: 'abc-123-original', name: 'tester-original' }, spec: { id: 'SOME_ID-original' } },
      });
      expect(resolved).toBe('index: 2, path: items');
    });

    test('provides repeated layout context to nested interpolations', () => {
      const { result } = renderHook(
        () => useInterpolationResolver(),
        buildWrapper([
          getInterpolationProviderWrapper({ user: createMockUser(), project: createMockProject() }),
          getRepeatedLayoutProviderWrapper({ index: 3, rootFieldPath: 'items.data' }),
          getRouterWrapper(),
        ])
      );

      const schema = {
        title: interpolate(
          ({ repeatedLayoutContext }) =>
            `Item ${repeatedLayoutContext?.index} of ${repeatedLayoutContext?.rootFieldPath}`
        ),
        path: interpolate('${repeatedLayoutContext.rootFieldPath}[${repeatedLayoutContext.index}]'),
      };

      const resolved = result.current(schema);

      expect(resolved).toEqual({
        title: 'Item 3 of items.data',
        path: 'items.data[3]',
      });
    });
  });

  describe('Property Exclusion', () => {
    test('excludes properties by key name', () => {
      const input = {
        shouldInterpolate: interpolate('${page.metadata.name}'),
        shouldExclude: interpolate('${page.metadata.name}'),
        nested: {
          shouldInterpolate: interpolate('${page.metadata.namespace}'),
          shouldExclude: interpolate('${page.metadata.namespace}'),
        },
      };

      const excludeProperty: ExclusionCheck = (key) => key === 'shouldExclude';
      const result = resolve(input, undefined, excludeProperty);

      expect(result.shouldInterpolate).toBe('tester');
      expect(result.shouldExclude).toBe(input.shouldExclude);
      expect(result.nested.shouldInterpolate).toBe('abc-123');
      expect(result.nested.shouldExclude).toBe(input.nested.shouldExclude);
    });

    test('excludes properties based on value type', () => {
      const input = {
        stringValue: interpolate('${page.metadata.name}'),
        functionValue: () => 'test',
        nested: {
          anotherString: interpolate('${page.metadata.namespace}'),
          anotherFunction: () => 'nested',
        },
      };

      const excludeFunctions: ExclusionCheck = (_key, value) => typeof value === 'function';

      const result = resolve(input, undefined, excludeFunctions);

      expect(result.stringValue).toBe('tester');
      expect(result.functionValue).toBe(input.functionValue);
      expect(result.nested.anotherString).toBe('abc-123');
      expect(result.nested.anotherFunction).toBe(input.nested.anotherFunction);
    });

    test('continues interpolation when exclusion function throws', () => {
      const input = {
        prop: interpolate('${page.metadata.name}'),
      };

      const faultyExclusion: ExclusionCheck = () => {
        throw new Error('Exclusion error');
      };

      const result = resolve(input, undefined, faultyExclusion);

      expect(result.prop).toBe('tester');
    });

    test('exclusion function receives correct parameters', () => {
      const input = {
        root: {
          nested: {
            target: interpolate('${page.metadata.name}'),
          },
        },
      };

      const capturedCalls: Array<{ key: string; value: any }> = [];
      const trackingExclusion: ExclusionCheck = (key, value) => {
        capturedCalls.push({ key, value });
        return false; // Don't exclude anything
      };

      resolve(input, undefined, trackingExclusion);

      expect(capturedCalls).toHaveLength(3);
      expect(capturedCalls[0]).toEqual({ key: 'root', value: input.root });
      expect(capturedCalls[1]).toEqual({ key: 'nested', value: input.root.nested });
      expect(capturedCalls[2]).toEqual({ key: 'target', value: input.root.nested.target });
    });
  });
});
