import { toClusterTarget } from '#core/config/entities/targets/inference-server-cluster-fields';

import type { ConnectableCluster } from '#core/config/entities/targets/types';

describe('toClusterTarget', () => {
  it('names the target after the cluster and copies its REST connection spec', () => {
    const cluster: ConnectableCluster = {
      metadata: { name: 'cluster-a', namespace: 'ma-system' },
      spec: {
        region: 'us-west',
        cluster: {
          case: 'kubernetes',
          value: {
            rest: {
              host: 'https://kubernetes.default.svc',
              port: '443',
              tokenTag: 'cluster-a-is-token',
              caDataTag: 'cluster-a-ca-data',
            },
          },
        },
      },
    };

    expect(toClusterTarget(cluster)).toEqual({
      clusterId: 'cluster-a',
      connection: {
        case: 'kubernetes',
        value: {
          host: 'https://kubernetes.default.svc',
          port: '443',
          tokenTag: 'cluster-a-is-token',
          caDataTag: 'cluster-a-ca-data',
        },
      },
    });
  });
});
