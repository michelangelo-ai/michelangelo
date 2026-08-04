import { CellType } from '#core/components/cell/constants';
import { readEnvironmentLabel } from '#core/utils/environment-utils';
import { getCrdUpdatedSeconds } from '#core/utils/timestamp-utils';

import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { ListViewConfig } from '#core/components/views/types';

/**
 * Numeric values for the `ModelKind` enum on the `Model` proto (model.proto).
 *
 * The generated proto client decodes enum fields to their numeric discriminant, not the
 * string enum name — e.g. `spec.kind` arrives in the browser as `2`, not
 * `"MODEL_KIND_REGRESSION"`. `typeTextMap` lookups below must be keyed accordingly (see the
 * same convention in `DEPLOYMENT_STAGE` / `DEPLOYMENT_STAGE_CELL` in
 * `config/entities/deployment/shared.ts`).
 */
export const MODEL_KIND = {
  INVALID: 0,
  CUSTOM: 1,
  REGRESSION: 2,
  BINARY_CLASSIFICATION: 3,
  MULTICLASS_CLASSIFICATION: 4,
  CLUSTERING: 5,
  LLM_COMPLETION: 6,
  LLM_CHAT_COMPLETION: 7,
  LLM_EMBEDDING: 8,
} as const;

/**
 * Human-readable labels for every `ModelKind` enum value on the `Model` proto (9 values).
 */
export const MODEL_KIND_TEXT_MAP: Record<number, string> = {
  [MODEL_KIND.INVALID]: 'Unknown',
  [MODEL_KIND.CUSTOM]: 'Custom',
  [MODEL_KIND.REGRESSION]: 'Regression',
  [MODEL_KIND.BINARY_CLASSIFICATION]: 'Binary Classification',
  [MODEL_KIND.MULTICLASS_CLASSIFICATION]: 'Multi-class Classification',
  [MODEL_KIND.CLUSTERING]: 'Clustering',
  [MODEL_KIND.LLM_COMPLETION]: 'LLM Completion',
  [MODEL_KIND.LLM_CHAT_COMPLETION]: 'LLM Chat',
  [MODEL_KIND.LLM_EMBEDDING]: 'LLM Embedding',
};

export const MODEL_CELL_CONFIG: ColumnConfig<object>[] = [
  {
    id: 'metadata.name',
    label: 'Model',
    items: [
      {
        id: 'metadata.name',
        url: '/${studio.projectId}/${studio.phase}/models/${data.metadata.name}',
      },
      {
        // spec.description is a free-text summary generated upstream by the training pipeline
        // (e.g. "model workflow=... git=... docker=..."). Rendered verbatim, no parsing.
        id: 'spec.description',
        type: CellType.DESCRIPTION,
      },
    ],
  },
  {
    // Model.spec has no environment field; environment is CRD-label-only metadata, read via
    // readEnvironmentLabel().
    id: 'metadata.labels',
    label: 'Environment',
    type: CellType.TEXT,
    accessor: (data: unknown) => {
      // cast: accessor receives unknown data; narrowing to expected proto shape for property
      // access; see #1425
      const labels = (data as { metadata?: { labels?: Record<string, string> } })?.metadata?.labels;
      return readEnvironmentLabel(labels) || null;
    },
  },
  {
    id: 'spec.modelFamily.name',
    label: 'Model Family',
    type: CellType.TEXT,
  },
  {
    // See MODEL_KIND_TEXT_MAP above for the full set of ModelKind display labels.
    id: 'spec.kind',
    label: 'Type',
    type: CellType.TYPE,
    typeTextMap: MODEL_KIND_TEXT_MAP,
  },
  {
    // Prefers the SpecUpdateTimestamp label, falls back to creationTimestamp; see
    // getCrdUpdatedSeconds().
    id: 'metadata',
    label: 'Last Updated',
    type: CellType.DATE,
    accessor: (data: unknown) => {
      // cast: accessor receives unknown data; narrowing to expected proto shape for property
      // access; see #1425
      const row = data as {
        metadata?: { labels?: Record<string, string>; creationTimestamp?: { seconds: number } };
      };
      return getCrdUpdatedSeconds(row);
    },
  },
];

export const MODEL_LIST_CONFIG: ListViewConfig<object> = {
  type: 'list',
  tableConfig: {
    columns: MODEL_CELL_CONFIG,
  },
};
