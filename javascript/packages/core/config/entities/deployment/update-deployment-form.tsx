import { DeploymentForm } from './deployment-form';

import type { ActionComponentProps } from '#core/components/actions/types';
import type { DeploymentRecord } from './types';

/** Update-mode entry point for the shared {@link DeploymentForm}. */
export const UpdateDeploymentForm = ({ record, onClose }: ActionComponentProps) => (
  // cast: record is Data from the action modal context; always a Deployment in this
  // entity config; see #1425
  <DeploymentForm mode="update" record={record as DeploymentRecord} onClose={onClose} />
);
