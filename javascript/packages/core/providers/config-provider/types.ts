import type {
  CategoryConfig,
  PhaseConfig,
  PhaseEntityConfig,
} from '#core/types/common/studio-types';

export type StudioConfigContextType = {
  categories: CategoryConfig[];
  getPhase: (phaseId: string) => PhaseConfig | undefined;
  getEntity: (phaseId: string, entityId: string) => PhaseEntityConfig | undefined;
};
