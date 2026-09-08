import type { ReactNode } from 'react';
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

export interface InjectedListOptions {
  fieldSelector?: string;
  labelSelector?: string;
}

export interface EntityTableProps<T extends object = object> {
  /** Service name for data fetching (e.g., 'pipeline' → 'ListPipeline') */
  service: QueryConfig['service'];
  tableConfig: TableConfig<T>;
  /** Unique ID for table state persistence */
  tableSettingsId: string;
  /** Pipeline types the owning phase restricts this entity's data to, if any */
  pipelineTypes?: string[];
  /** Rendered in the trailing section of the table's search/filter action bar */
  trailingActions?: ReactNode;
}
