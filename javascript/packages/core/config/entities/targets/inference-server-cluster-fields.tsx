import { useMemo } from 'react';

import { SelectField } from '#core/components/form/fields/select/select-field';
import { required } from '#core/components/form/validation/validators';
import { useStudioQuery } from '#core/hooks/use-studio-query';
import { CLUSTER_REGISTRY_NAMESPACE } from './shared';

import type { SelectOption } from '#core/components/form/fields/select/types';
import type { ClusterListResult, ClusterTarget, RegisteredCluster } from './types';

/** Multi-select of registered compute clusters, stored on the form as `clusterIds`. */
export function InferenceServerClusterFields({
  clusters,
  isLoading,
}: {
  clusters: RegisteredCluster[];
  isLoading: boolean;
}) {
  const options = useMemo<SelectOption<string>[]>(
    () =>
      clusters.map((cluster) => {
        const location = [cluster.spec?.region, cluster.spec?.zone].filter(Boolean).join(' / ');
        return {
          id: cluster.metadata.name,
          label: location ? `${cluster.metadata.name} (${location})` : cluster.metadata.name,
        };
      }),
    [clusters]
  );

  return (
    <SelectField<string>
      name="clusterIds"
      label="Cluster targets"
      multi
      required
      validate={required()}
      options={options}
      isLoading={isLoading}
      searchable
      clearable={false}
      caption="Clusters this server is provisioned on. Every model deployed to it runs on all of them."
    />
  );
}

/** Lists the compute clusters operators have registered in the control plane. */
export function useRegisteredClusters() {
  const query = useStudioQuery<ClusterListResult>({
    queryName: 'ListCluster',
    serviceOptions: { namespace: CLUSTER_REGISTRY_NAMESPACE },
  });
  const clusters = useMemo(() => query.data?.clusterList.items ?? [], [query.data]);
  return { clusters, isLoading: query.isLoading };
}

/**
 * Builds `spec.clusterTargets` from the picked cluster names by copying each registered
 * Cluster's connection spec. Clusters without a REST connection are skipped.
 */
export function toClusterTargets(
  clusterIds: string[],
  clusters: RegisteredCluster[]
): ClusterTarget[] {
  const byName = new Map(clusters.map((cluster) => [cluster.metadata.name, cluster]));
  const targets: ClusterTarget[] = [];
  for (const clusterId of clusterIds) {
    const rest = byName.get(clusterId)?.spec?.cluster?.value?.rest;
    if (!rest?.host) continue;
    targets.push({
      clusterId,
      connection: {
        case: 'kubernetes',
        value: {
          host: rest.host,
          port: rest.port ?? '',
          tokenTag: rest.tokenTag ?? '',
          caDataTag: rest.caDataTag ?? '',
        },
      },
    });
  }
  return targets;
}
