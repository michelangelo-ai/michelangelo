import { useCallback, useMemo } from 'react';

import { ConfigContext } from './config-context';

import type { StudioConfig } from '#core/types/common/studio-types';

export function ConfigProvider({
  children,
  config,
}: {
  children: React.ReactNode;
  config: StudioConfig;
}) {
  const getPhase = useCallback(
    (phaseId: string) => config.categories.flatMap((c) => c.phases).find((p) => p.id === phaseId),
    [config.categories]
  );

  const getEntity = useCallback(
    (phaseId: string, entityId: string) =>
      getPhase(phaseId)?.entities.find((e) => e.id === entityId),
    [getPhase]
  );

  const value = useMemo(
    () => ({ categories: config.categories, getPhase, getEntity }),
    [config.categories, getPhase, getEntity]
  );

  return <ConfigContext.Provider value={value}>{children}</ConfigContext.Provider>;
}
