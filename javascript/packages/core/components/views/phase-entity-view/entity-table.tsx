import { useState } from 'react';
import { Segment, SegmentedControl } from 'baseui/segmented-control';

import { useLocalStorageTableState } from '#core/components/table/plugins/state-persistence/use-local-storage-table-state';
import { Table } from '#core/components/table/table';
import { adaptTableConfigToTableProps } from '#core/components/views/utils/table-view-adapter';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioQuery } from '#core/hooks/use-studio-query';
import { capitalizeFirstLetter } from '#core/utils/string-utils';
import { injectListOptions } from './utils/inject-list-options';

import type { ListViewVariant } from '#core/components/views/types';
import type { EntityTableProps } from './types';

/**
 * Generic table component that renders entity data using configuration-driven queries.
 *
 * When `variants` are provided, a segmented control in the action bar switches between
 * data sources (e.g. pipelines vs. their revisions). The first variant is active on mount.
 *
 * @example
 * ```tsx
 * // Renders pipelines table with query 'ListPipeline' and data from 'pipelineList.items'
 * <EntityTable
 *   service="pipeline"
 *   tableConfig={{ columns: PIPELINE_COLUMNS, disableSearch: true }}
 *   tableSettingsId="train-pipelines"
 * />
 * ```
 */
export function EntityTable<T extends object = object>({
  service: entityService,
  tableConfig: entityTableConfig,
  tableSettingsId: entityTableSettingsId,
  pipelineTypes,
  trailingActions,
  variants,
}: EntityTableProps<T>) {
  const { projectId } = useStudioParams('base');
  const [activeVariantId, setActiveVariantId] = useState<string>();

  const activeVariant = resolveActiveVariant(variants, activeVariantId);
  const service = activeVariant?.service ?? entityService;
  const tableConfig = activeVariant?.tableConfig ?? entityTableConfig;
  const tableSettingsId = activeVariant
    ? `${entityTableSettingsId}/${activeVariant.id}`
    : entityTableSettingsId;

  const listOptions = injectListOptions(service, pipelineTypes);

  const { data, isLoading, error } = useStudioQuery<Record<`${string}List`, { items: T[] }>>({
    queryName: `List${capitalizeFirstLetter(service)}`,
    serviceOptions: {
      namespace: projectId,
      ...(listOptions && { listOptions }),
    },
  });

  const entityTableState = useLocalStorageTableState({
    filterSettingsId: `${projectId}/${tableSettingsId}`,
    tableSettingsId,
  });

  const tableProps = adaptTableConfigToTableProps<T>(tableConfig, {
    data: data?.[`${service}List`]?.items ?? [],
    loading: isLoading,
    error: error ?? undefined,
  });

  const handleVariantChange = ({ activeKey }: { activeKey: React.Key }) =>
    setActiveVariantId(String(activeKey));

  const variantSelector = variants && activeVariant && (
    <SegmentedControl
      activeKey={activeVariant.id}
      onChange={handleVariantChange}
      overrides={VARIANT_SELECTOR_OVERRIDES}
    >
      {variants.map((variant) => (
        <Segment key={variant.id} label={variant.label} />
      ))}
    </SegmentedControl>
  );

  return (
    <Table
      {...tableProps}
      actionBarConfig={{
        ...tableProps.actionBarConfig,
        middle: variantSelector,
        trailing: trailingActions,
      }}
      state={entityTableState}
    />
  );
}

/** Falls back to the first variant when no selection has been made yet. */
function resolveActiveVariant<T extends object>(
  variants: ListViewVariant<T>[] | undefined,
  requestedId: string | undefined
): ListViewVariant<T> | undefined {
  if (!variants?.length) return undefined;
  return variants.find((variant) => variant.id === requestedId) ?? variants[0];
}

const VARIANT_SELECTOR_OVERRIDES = {
  Root: { style: { borderRadius: '24px' } },
  Active: { style: { borderRadius: '24px' } },
  SegmentList: { style: { minHeight: 'unset' } },
};
