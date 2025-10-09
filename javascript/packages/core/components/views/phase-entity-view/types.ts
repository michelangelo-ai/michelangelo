import type { ListViewConfig, TableConfig, ViewConfig } from '#core/components/views/types';
import type { PhaseConfig, PhaseEntityConfig } from '#core/types/common/studio-types';
import type { QueryConfig } from '#core/types/query-types';

export type ListableEntity<T extends object = object> = PhaseEntityConfig<T> & {
  state: 'active';
  views: ViewConfig<T>[] & { 0: ListViewConfig<T> };
};

export interface PhaseEntityViewProps<T extends object = object> {
  /**
   * Listable entities passed separately via entities prop; phaseConfig is used for
   * other metadata
   */
  phaseConfig: Omit<PhaseConfig, 'entities'>;

  entities: ListableEntity<T>[];
}

export interface EntityTableProps<T extends object = object> {
  /** Service name for data fetching (e.g., 'pipeline' → 'ListPipeline') */
  service: QueryConfig['service'];
  tableConfig: TableConfig<T>;
  /** Unique ID for table state persistence */
  tableSettingsId: string;
}
