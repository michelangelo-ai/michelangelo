import type { ListViewConfig, TableConfig, ViewConfig } from '#core/components/views/types';
import type { PhaseEntityConfig } from '#core/types/common/studio-types';

export type ListableEntity<T extends object = object> = PhaseEntityConfig<T> & {
  state: 'active';
  views: ViewConfig<T>[] & { 0: ListViewConfig<T> };
};

export interface PhaseEntityViewProps<T extends object = object> {
  phaseId: string;
  entities: ListableEntity<T>[];
}

export interface EntityTableProps<T extends object = object> {
  /** Service name for data fetching (e.g., 'pipeline' → 'ListPipeline') */
  service: string;
  tableConfig: TableConfig<T>;
  /** Unique ID for table state persistence */
  tableSettingsId: string;
}
