import { useContext } from 'react';

import { ConfigContext } from './config-context';

/**
 * Access the studio configuration — categories, phases, and entities — provided
 * to `CoreApp` via the `config` prop.
 *
 * Use `getPhase` and `getEntity` for lookups by URL param. Both return
 * `undefined` when the ID doesn't match any configured item.
 *
 * @example
 * ```ts
 * const { categories, getPhase, getEntity } = useStudioConfig();
 *
 * const phase = getPhase('train');
 * if (!phase) return <ErrorView title="Phase not found" />;
 *
 * const entity = getEntity('train', 'pipelines');
 * ```
 */
export function useStudioConfig() {
  const context = useContext(ConfigContext);
  if (!context) {
    throw new Error('useStudioConfig must be used within a ConfigProvider');
  }
  return context;
}
