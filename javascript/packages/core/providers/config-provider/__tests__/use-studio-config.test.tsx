import { renderHook } from '@testing-library/react';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getConfigProviderWrapper } from '#core/test/wrappers/get-config-provider-wrapper';
import { useStudioConfig } from '../use-studio-config';

describe('useStudioConfig', () => {
  test('returns categories from config', () => {
    const { result } = renderHook(
      () => useStudioConfig(),
      buildWrapper([
        getConfigProviderWrapper({
          categories: [
            { id: 'core-ml', name: 'Core ML', phases: [] },
            { id: 'gen-ai', name: 'Gen AI', phases: [] },
          ],
        }),
      ])
    );

    expect(result.current.categories).toHaveLength(2);
    expect(result.current.categories[0].id).toBe('core-ml');
    expect(result.current.categories[1].id).toBe('gen-ai');
  });

  test('getPhase returns matching phase across categories', () => {
    const { result } = renderHook(
      () => useStudioConfig(),
      buildWrapper([
        getConfigProviderWrapper({
          categories: [
            {
              id: 'core-ml',
              name: 'Core ML',
              phases: [
                { id: 'train', icon: 'train', name: 'Train', state: 'active', entities: [] },
              ],
            },
          ],
        }),
      ])
    );

    expect(result.current.getPhase('train')).toEqual(
      expect.objectContaining({ id: 'train', name: 'Train' })
    );
  });

  test('getPhase returns undefined for unknown phase', () => {
    const { result } = renderHook(
      () => useStudioConfig(),
      buildWrapper([
        getConfigProviderWrapper({
          categories: [
            {
              id: 'core-ml',
              name: 'Core ML',
              phases: [
                { id: 'train', icon: 'train', name: 'Train', state: 'active', entities: [] },
              ],
            },
          ],
        }),
      ])
    );

    expect(result.current.getPhase('nonexistent')).toBeUndefined();
  });

  test('getEntity returns matching entity within a phase', () => {
    const { result } = renderHook(
      () => useStudioConfig(),
      buildWrapper([
        getConfigProviderWrapper({
          categories: [
            {
              id: 'core-ml',
              name: 'Core ML',
              phases: [
                {
                  id: 'train',
                  icon: 'train',
                  name: 'Train',
                  state: 'active',
                  entities: [
                    {
                      id: 'pipelines',
                      name: 'Pipelines',
                      service: 'pipeline',
                      state: 'active',
                      views: [],
                    },
                  ],
                },
              ],
            },
          ],
        }),
      ])
    );

    expect(result.current.getEntity('train', 'pipelines')).toEqual(
      expect.objectContaining({ id: 'pipelines', service: 'pipeline' })
    );
  });

  test('getEntity returns undefined for unknown entity in valid phase', () => {
    const { result } = renderHook(
      () => useStudioConfig(),
      buildWrapper([
        getConfigProviderWrapper({
          categories: [
            {
              id: 'core-ml',
              name: 'Core ML',
              phases: [
                {
                  id: 'train',
                  icon: 'train',
                  name: 'Train',
                  state: 'active',
                  entities: [
                    {
                      id: 'pipelines',
                      name: 'Pipelines',
                      service: 'pipeline',
                      state: 'active',
                      views: [],
                    },
                  ],
                },
              ],
            },
          ],
        }),
      ])
    );

    expect(result.current.getEntity('train', 'nonexistent')).toBeUndefined();
  });

  test('getEntity returns undefined when phase does not exist', () => {
    const { result } = renderHook(
      () => useStudioConfig(),
      buildWrapper([getConfigProviderWrapper({ categories: [] })])
    );

    expect(result.current.getEntity('nonexistent', 'pipelines')).toBeUndefined();
  });

  test('throws when used outside ConfigProvider', () => {
    expect(() => {
      renderHook(() => useStudioConfig());
    }).toThrow('useStudioConfig must be used within a ConfigProvider');
  });
});
