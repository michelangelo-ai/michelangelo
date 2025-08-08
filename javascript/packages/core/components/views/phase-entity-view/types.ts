import type { ListViewConfig, ViewConfig } from '#core/components/views/types';
import type { PhaseEntityConfig } from '#core/types/common/studio-types';

export type ListableEntity = PhaseEntityConfig & {
  state: 'active';
  views: ViewConfig[] & { 0: ListViewConfig };
};

export interface PhaseEntityViewProps {
  phaseId: string;
  entities: ListableEntity[];
}

export interface EntityTableProps {
  /** Service name used to construct RPC query (e.g., 'pipeline' → 'ListPipeline') */
  service: string;
  listViewConfig: ListViewConfig;
  /** Unique ID for table state persistence */
  tableSettingsId: string;
}
