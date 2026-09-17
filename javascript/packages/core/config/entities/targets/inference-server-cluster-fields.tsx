import { SelectField } from '#core/components/form/fields/select/select-field';
import { required } from '#core/components/form/validation/validators';
import { useStudioQuery } from '#core/hooks/use-studio-query';
import { CLUSTER_REGISTRY_NAMESPACE } from './shared';

import type { SelectOption } from '#core/components/form/fields/select/types';
import type {
  ClusterListResult,
  ClusterTarget,
  ConnectableCluster,
  RegisteredCluster,
} from './types';

export function InferenceServerClusterFields({
  clusters,
  isLoading,
}: {
  clusters: ConnectableCluster[];
  isLoading: boolean;
}) {
  const options: SelectOption<ClusterTarget>[] = clusters.map((cluster) => {
    const location = [cluster.spec.region, cluster.spec.zone].filter(Boolean).join(' / ');
    return {
      id: toClusterTarget(cluster),
      label: location ? `${cluster.metadata.name} (${location})` : cluster.metadata.name,
    };
  });

  return (
    <SelectField<ClusterTarget>
      name="spec.clusterTargets"
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

/**
 * Lists the compute clusters operators have registered in the control plane. Clusters without
 * a REST connection can't become a cluster target, so they are filtered out here to keep the
 * options the user sees in step with what can be submitted.
 */
export function useRegisteredClusters() {
  const query = useStudioQuery<ClusterListResult>({
    queryName: 'ListCluster',
    serviceOptions: { namespace: CLUSTER_REGISTRY_NAMESPACE },
  });
  const clusters = (query.data?.clusterList.items ?? []).filter(hasRestConnection);
  return { clusters, isLoading: query.isLoading };
}

/** Converts a registered Cluster into the `spec.clusterTargets` entry that points at it. */
export function toClusterTarget(cluster: ConnectableCluster): ClusterTarget {
  return {
    clusterId: cluster.metadata.name,
    connection: { case: 'kubernetes', value: cluster.spec.cluster.value.rest },
  };
}

function hasRestConnection(cluster: RegisteredCluster): cluster is ConnectableCluster {
  return cluster.spec?.cluster?.value?.rest !== undefined;
}
