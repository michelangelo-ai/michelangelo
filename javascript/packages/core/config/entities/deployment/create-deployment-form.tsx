import { DeploymentForm } from './deployment-form';

import type { CreateActionComponentProps } from '#core/components/actions/types';

/** Create-mode entry point for the shared {@link DeploymentForm}. */
export const CreateDeploymentForm = ({ onClose }: CreateActionComponentProps) => (
  <DeploymentForm mode="create" onClose={onClose} />
);
