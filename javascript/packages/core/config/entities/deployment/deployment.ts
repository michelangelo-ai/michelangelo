import { ActionHierarchy } from '#core/components/actions/types';
import { interpolate } from '#core/interpolation/interpolate';
import { TimeZone } from '#core/types/time-types';
import { getCrdUpdatedSeconds } from '#core/utils/crd-utils';
import { timestampToString } from '#core/utils/time-utils';
import { CreateDeploymentForm } from './create-deployment-form';
import { DEPLOYMENT_DETAIL_CONFIG } from './detail';
import { DEPLOYMENT_LIST_CONFIG } from './list';

import type { PhaseEntityConfig } from '#core/types/common/studio-types';
import type { DeploymentRecord } from './types';

const LAST_PREDICTION_ANNOTATION = 'deployment.michelangelo/last-prediction-timestamp';

const isRetirable = (record: unknown) => {
  // cast: record is unknown from the action predicate context; always a Deployment in this
  // entity config; see #1425
  const deployment = record as DeploymentRecord;
  return !!(deployment.spec?.desiredRevision ?? deployment.status?.candidateRevision);
};

const retireModalBody = (record: unknown) => {
  // cast: record is unknown from interpolation context; always a Deployment in this entity
  // config; see #1425
  const deployment = record as DeploymentRecord;
  const deployed = timestampToString(getCrdUpdatedSeconds(deployment), TimeZone.Local);
  const lastUsed = timestampToString(
    deployment.metadata?.annotations?.[LAST_PREDICTION_ANNOTATION],
    TimeZone.Local
  );
  return `Deployed at: **${deployed ?? 'N/A'}**\n\nLast used at: **${lastUsed ?? 'N/A'}**\n\nThis process might take a few minutes.`;
};

export const DEPLOYMENT_ENTITY_CONFIG: PhaseEntityConfig = {
  id: 'deployments',
  name: 'Deployments',
  service: 'deployment',
  state: 'active',
  views: [DEPLOYMENT_LIST_CONFIG, DEPLOYMENT_DETAIL_CONFIG],
  actions: [
    {
      display: { label: 'Retire', icon: 'circleX' },
      hierarchy: ActionHierarchy.TERTIARY,
      disabled: [
        {
          condition: interpolate(({ data }) => !isRetirable(data)),
          message: 'Deployment has already been retired',
        },
      ],
      operation: {
        type: 'mutation',
        mutation: {
          mutationName: 'UpdateDeployment',
          middleware: {
            operations: [{ destination: 'spec.desiredRevision', transformation: 'unset' }],
          },
          successOperations: [
            {
              type: 'invalidate',
              targets: ['GetDeployment', 'ListDeployment'],
              delayMs: 2000,
            },
            {
              type: 'toast',
              message: interpolate(
                'Retirement for deployment ${response.deployment.metadata.name} has begun'
              ),
            },
          ],
        },
      },
      modal: {
        type: 'confirm',
        header: {
          title: interpolate(
            ({ data }) =>
              // cast: data is unknown from interpolation context; always a Deployment in this
              // entity config; see #1425
              `Are you sure you want to retire ${(data as DeploymentRecord).metadata?.name}`
          ),
        },
        body: interpolate(({ data }) => retireModalBody(data)),
        button: { label: 'Yes, retire', icon: 'check' },
      },
    },
  ],
  createAction: {
    display: { label: 'Create deployment', icon: 'plus' },
    component: CreateDeploymentForm,
  },
};
