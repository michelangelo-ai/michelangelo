import { renderHook } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { useSchemaMiddleware } from '#core/hooks/use-schema-middleware/use-schema-middleware';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';

import type { StudioParamsBase } from '#core/hooks/routing/use-studio-params/types';

describe('useSchemaMiddleware', () => {
  it('returns the original data unchanged when schema is null', () => {
    const { result } = renderHook(() => useSchemaMiddleware(null), {
      wrapper: getRouterWrapper({ location: '/test-project/train/model' }),
    });
    const data = { metadata: { name: 'foo' } };
    expect(result.current.applyMiddleware(data)).toEqual(data);
  });

  it('returns the original data unchanged when schema is undefined', () => {
    const { result } = renderHook(() => useSchemaMiddleware(undefined), {
      wrapper: getRouterWrapper({ location: '/test-project/train/model' }),
    });
    const data = { metadata: { name: 'foo' } };
    expect(result.current.applyMiddleware(data)).toEqual(data);
  });

  it('returns the original data unchanged when operations is empty', () => {
    const { result } = renderHook(() => useSchemaMiddleware({ operations: [] }), {
      wrapper: getRouterWrapper({ location: '/test-project/train/model' }),
    });
    const data = { metadata: { name: 'foo' } };
    expect(result.current.applyMiddleware(data)).toEqual(data);
  });

  it('sets destination to default value when source is absent', () => {
    const { result } = renderHook(
      () => useSchemaMiddleware({ operations: [{ destination: 'spec.action', default: 1 }] }),
      { wrapper: getRouterWrapper({ location: '/test-project/train/model' }) }
    );
    expect(result.current.applyMiddleware({ spec: {} })).toMatchObject({ spec: { action: 1 } });
  });

  it('sets destination to default value when source path resolves to nil', () => {
    const { result } = renderHook(
      () =>
        useSchemaMiddleware({
          operations: [{ source: 'spec.missing', destination: 'spec.action', default: 'fallback' }],
        }),
      { wrapper: getRouterWrapper({ location: '/test-project/train/model' }) }
    );
    expect(result.current.applyMiddleware({ spec: {} })).toMatchObject({
      spec: { action: 'fallback' },
    });
  });

  it('does not write destination when source is nil and no default is defined', () => {
    const { result } = renderHook(
      () =>
        useSchemaMiddleware({
          operations: [{ source: 'spec.missing', destination: 'spec.action' }],
        }),
      { wrapper: getRouterWrapper({ location: '/test-project/train/model' }) }
    );
    expect(result.current.applyMiddleware({ spec: {} })).toEqual({ spec: {} });
  });

  it('applies transformation function to source value', () => {
    const { result } = renderHook(
      () =>
        useSchemaMiddleware({
          operations: [
            {
              source: 'metadata.name',
              destination: 'metadata.displayName',
              transformation: (name) => (name as string).toUpperCase(),
            },
          ],
        }),
      { wrapper: getRouterWrapper({ location: '/test-project/train/model' }) }
    );
    expect(result.current.applyMiddleware({ metadata: { name: 'foo' } })).toMatchObject({
      metadata: { name: 'foo', displayName: 'FOO' },
    });
  });

  it('uses default instead of transformation when source is nil', () => {
    const { result } = renderHook(
      () =>
        useSchemaMiddleware({
          operations: [
            {
              source: 'metadata.missing',
              destination: 'metadata.displayName',
              default: 'unnamed',
              transformation: (name) => (name as string).toUpperCase(),
            },
          ],
        }),
      { wrapper: getRouterWrapper({ location: '/test-project/train/model' }) }
    );
    expect(result.current.applyMiddleware({ metadata: {} })).toMatchObject({
      metadata: { displayName: 'unnamed' },
    });
  });

  it('unsets destination path when transformation is "unset"', () => {
    const { result } = renderHook(
      () =>
        useSchemaMiddleware({
          operations: [{ destination: 'spec.deprecated', transformation: 'unset' }],
        }),
      { wrapper: getRouterWrapper({ location: '/test-project/train/model' }) }
    );
    expect(result.current.applyMiddleware({ spec: { deprecated: true, keep: 'yes' } })).toEqual({
      spec: { keep: 'yes' },
    });
  });

  it('does not write destination when source is present but no transformation is defined', () => {
    const { result } = renderHook(
      () =>
        useSchemaMiddleware({
          operations: [{ source: 'spec.action', destination: 'spec.result' }],
        }),
      { wrapper: getRouterWrapper({ location: '/test-project/train/model' }) }
    );
    expect(result.current.applyMiddleware({ spec: { action: 'run' } })).toEqual({
      spec: { action: 'run' },
    });
  });

  describe('falsy source value handling', () => {
    const schema = {
      operations: [
        {
          source: 'source',
          destination: 'destination',
          default: 'defaultValue',
          transformation: (v: unknown) => v,
        },
      ],
    };

    it('uses default when source value is null', () => {
      const { result } = renderHook(() => useSchemaMiddleware(schema), {
        wrapper: getRouterWrapper({ location: '/test-project/train/model' }),
      });
      expect(result.current.applyMiddleware({ source: null })).toEqual({
        source: null,
        destination: 'defaultValue',
      });
    });

    it('copies source to destination when source value is false', () => {
      const { result } = renderHook(() => useSchemaMiddleware(schema), {
        wrapper: getRouterWrapper({ location: '/test-project/train/model' }),
      });
      expect(result.current.applyMiddleware({ source: false })).toEqual({
        source: false,
        destination: false,
      });
    });

    it('copies source to destination when source value is 0', () => {
      const { result } = renderHook(() => useSchemaMiddleware(schema), {
        wrapper: getRouterWrapper({ location: '/test-project/train/model' }),
      });
      expect(result.current.applyMiddleware({ source: 0 })).toEqual({
        source: 0,
        destination: 0,
      });
    });
  });

  it('calls default function with studio params when source is nil', () => {
    const { result } = renderHook(
      () =>
        useSchemaMiddleware({
          operations: [
            {
              destination: 'metadata.namespace',
              default: ({ studio }: { studio: StudioParamsBase }) => studio.projectId,
            },
          ],
        }),
      { wrapper: getRouterWrapper({ location: '/my-project/train/model' }) }
    );
    expect(result.current.applyMiddleware({ metadata: {} })).toMatchObject({
      metadata: { namespace: 'my-project' },
    });
  });

  it('applies all operations in order', () => {
    const { result } = renderHook(
      () =>
        useSchemaMiddleware({
          operations: [
            { destination: 'spec.action', default: 1 },
            { destination: 'spec.kill', default: true },
            { destination: 'spec.deprecated', transformation: 'unset' },
          ],
        }),
      { wrapper: getRouterWrapper({ location: '/test-project/train/model' }) }
    );
    expect(result.current.applyMiddleware({ spec: { deprecated: true } })).toEqual({
      spec: { action: 1, kill: true },
    });
  });

  it('reads operation source from sourceFromObject instead of data', () => {
    const { result } = renderHook(
      () =>
        useSchemaMiddleware({
          operations: [
            {
              source: 'spec.name',
              destination: 'spec.displayName',
              transformation: (v) => (v as string).toUpperCase(),
            },
          ],
        }),
      { wrapper: getRouterWrapper({ location: '/test-project/train/model' }) }
    );
    const data = { spec: { name: 'from-data' } };
    const sourceFromObject = { spec: { name: 'from-source' } };
    expect(result.current.applyMiddleware(data, { sourceFromObject })).toMatchObject({
      spec: { displayName: 'FROM-SOURCE' },
    });
  });

  it('writes the result to data, not sourceFromObject', () => {
    const { result } = renderHook(
      () =>
        useSchemaMiddleware({
          operations: [
            {
              source: 'value',
              destination: 'derived',
              transformation: (v) => v,
            },
          ],
        }),
      { wrapper: getRouterWrapper({ location: '/test-project/train/model' }) }
    );
    const data = { existing: true };
    const sourceFromObject = { value: 'hello' };
    const output = result.current.applyMiddleware(data, { sourceFromObject });
    expect(output).toMatchObject({ existing: true, derived: 'hello' });
    expect(output).not.toHaveProperty('value');
  });

  it('falls back to default when source path is absent in sourceFromObject', () => {
    const { result } = renderHook(
      () =>
        useSchemaMiddleware({
          operations: [{ source: 'spec.missing', destination: 'spec.action', default: 'fallback' }],
        }),
      { wrapper: getRouterWrapper({ location: '/test-project/train/model' }) }
    );
    expect(
      result.current.applyMiddleware({ spec: {} }, { sourceFromObject: { spec: {} } })
    ).toMatchObject({ spec: { action: 'fallback' } });
  });

  it('merges scaffold YAML into the record before operations run', () => {
    const { result } = renderHook(
      () =>
        useSchemaMiddleware({
          scaffold: 'spec:\n  proto_module: default-module',
        }),
      { wrapper: getRouterWrapper({ location: '/test-project/train/model' }) }
    );
    expect(result.current.applyMiddleware({ spec: {} })).toMatchObject({
      spec: { proto_module: 'default-module' },
    });
  });

  it('data values win over scaffold defaults', () => {
    const { result } = renderHook(
      () =>
        useSchemaMiddleware({
          scaffold: 'spec:\n  proto_module: scaffold-value',
        }),
      { wrapper: getRouterWrapper({ location: '/test-project/train/model' }) }
    );
    expect(
      result.current.applyMiddleware({ spec: { proto_module: 'existing-value' } })
    ).toMatchObject({ spec: { proto_module: 'existing-value' } });
  });

  it('applies the matching subType scaffold from scaffoldBySubType', () => {
    const { result } = renderHook(
      () =>
        useSchemaMiddleware({
          subTypePath: 'spec.subType',
          scaffoldBySubType: {
            regression: 'spec:\n  scaffoldedId: regression-scaffold',
            classification: 'spec:\n  scaffoldedId: classification-scaffold',
          },
        }),
      { wrapper: getRouterWrapper({ location: '/test-project/train/model' }) }
    );
    expect(result.current.applyMiddleware({ spec: { subType: 'regression' } })).toMatchObject({
      spec: { scaffoldedId: 'regression-scaffold' },
    });
    expect(result.current.applyMiddleware({ spec: { subType: 'classification' } })).toMatchObject({
      spec: { scaffoldedId: 'classification-scaffold' },
    });
  });

  it('operations can source values set by scaffold', () => {
    const { result } = renderHook(
      () =>
        useSchemaMiddleware({
          scaffold: 'spec:\n  scaffoldedProp: scaffoldedValue',
          operations: [
            {
              source: 'spec.scaffoldedProp',
              destination: 'spec.transformed',
              transformation: (v) => `${v as string}-transformed`,
            },
          ],
        }),
      { wrapper: getRouterWrapper({ location: '/test-project/train/model' }) }
    );
    expect(result.current.applyMiddleware({ spec: {} })).toMatchObject({
      spec: { scaffoldedProp: 'scaffoldedValue', transformed: 'scaffoldedValue-transformed' },
    });
  });

  it('throws a descriptive error for invalid YAML', () => {
    const { result } = renderHook(
      () => useSchemaMiddleware({ scaffold: 'key: [unclosed' }),
      { wrapper: getRouterWrapper({ location: '/test-project/train/model' }) }
    );
    expect(() => result.current.applyMiddleware({})).toThrow(
      'Request requires scaffolding, but found invalid YAML scaffold'
    );
  });

  describe('subTypes filtering', () => {
    const subTypePath = 'spec.subType';

    it('runs an operation only for its declared subTypes', () => {
      const { result } = renderHook(
        () =>
          useSchemaMiddleware({
            subTypePath,
            operations: [
              {
                subTypes: ['regression'],
                destination: 'spec.regressionOnly',
                default: 'regression-default',
              },
            ],
          }),
        { wrapper: getRouterWrapper({ location: '/test-project/train/model' }) }
      );
      expect(result.current.applyMiddleware({ spec: { subType: 'regression' } })).toMatchObject({
        spec: { regressionOnly: 'regression-default' },
      });
      expect(result.current.applyMiddleware({ spec: { subType: 'classification' } })).toEqual({
        spec: { subType: 'classification' },
      });
    });

    it('runs an operation for all listed subTypes', () => {
      const { result } = renderHook(
        () =>
          useSchemaMiddleware({
            subTypePath,
            operations: [
              {
                subTypes: ['classification', 'regression'],
                destination: 'spec.shared',
                default: 'shared-default',
              },
            ],
          }),
        { wrapper: getRouterWrapper({ location: '/test-project/train/model' }) }
      );
      expect(result.current.applyMiddleware({ spec: { subType: 'classification' } })).toMatchObject(
        { spec: { shared: 'shared-default' } }
      );
      expect(result.current.applyMiddleware({ spec: { subType: 'regression' } })).toMatchObject({
        spec: { shared: 'shared-default' },
      });
    });
  });

  it('does not mutate the original record', () => {
    const { result } = renderHook(
      () => useSchemaMiddleware({ operations: [{ destination: 'spec.action', default: 1 }] }),
      { wrapper: getRouterWrapper({ location: '/test-project/train/model' }) }
    );
    const original = { spec: {} };
    result.current.applyMiddleware(original);
    expect(original).toEqual({ spec: {} });
  });
});
