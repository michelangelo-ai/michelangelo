import { toClusterTargets } from '#core/config/entities/targets/inference-server-cluster-fields';

import type { RegisteredCluster } from '#core/config/entities/targets/types';

function buildCluster(overrides: Partial<RegisteredCluster> = {}): RegisteredCluster {
  return {
    metadata: { name: 'test-cluster', namespace: 'ma-system' },
    spec: {
      cluster: {
        case: 'kubernetes',
        value: {
          rest: {
            host: 'https://kubernetes.default.svc',
            port: '443',
            tokenTag: 'cluster-test-cluster-is-token',
            caDataTag: 'cluster-test-cluster-ca-data',
          },
        },
      },
    },
    ...overrides,
  };
}

describe('toClusterTargets', () => {
  it('copies the connection spec of each picked cluster', () => {
    const clusters = [
      buildCluster({ metadata: { name: 'cluster-a', namespace: 'ma-system' } }),
      buildCluster({ metadata: { name: 'cluster-b', namespace: 'ma-system' } }),
    ];

    expect(toClusterTargets(['cluster-a'], clusters)).toEqual([
      {
        clusterId: 'cluster-a',
        connection: {
          case: 'kubernetes',
          value: {
            host: 'https://kubernetes.default.svc',
            port: '443',
            tokenTag: 'cluster-test-cluster-is-token',
            caDataTag: 'cluster-test-cluster-ca-data',
          },
        },
      },
    ]);
  });

  it('preserves the picked order and supports more than one target', () => {
    const clusters = [
      buildCluster({ metadata: { name: 'cluster-a', namespace: 'ma-system' } }),
      buildCluster({ metadata: { name: 'cluster-b', namespace: 'ma-system' } }),
    ];

    const targets = toClusterTargets(['cluster-b', 'cluster-a'], clusters);

    expect(targets.map((target) => target.clusterId)).toEqual(['cluster-b', 'cluster-a']);
  });

  it('skips a picked cluster that has no REST connection', () => {
    const clusters = [
      buildCluster({
        metadata: { name: 'no-connection', namespace: 'ma-system' },
        spec: {},
      }),
    ];

    expect(toClusterTargets(['no-connection'], clusters)).toEqual([]);
  });

  it('skips a picked cluster that is not in the registered list', () => {
    expect(toClusterTargets(['unknown-cluster'], [])).toEqual([]);
  });

  it('defaults missing port and secret tags to empty strings', () => {
    const clusters = [
      buildCluster({
        metadata: { name: 'partial', namespace: 'ma-system' },
        spec: {
          cluster: {
            case: 'kubernetes',
            value: { rest: { host: 'https://partial.example.com' } },
          },
        },
      }),
    ];

    expect(toClusterTargets(['partial'], clusters)).toEqual([
      {
        clusterId: 'partial',
        connection: {
          case: 'kubernetes',
          value: {
            host: 'https://partial.example.com',
            port: '',
            tokenTag: '',
            caDataTag: '',
          },
        },
      },
    ]);
  });
});
