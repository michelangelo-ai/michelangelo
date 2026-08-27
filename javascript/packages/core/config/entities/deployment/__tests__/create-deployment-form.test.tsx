import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { CreateDeploymentForm } from '#core/config/entities/deployment/create-deployment-form';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { getInterpolationProviderWrapper } from '#core/test/wrappers/get-interpolation-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import {
  createQueryMockRouter,
  getServiceProviderWrapper,
} from '#core/test/wrappers/get-service-provider-wrapper';

describe('CreateDeploymentForm', () => {
  it('defaults "Type of deployment" to Online and does not allow switching it', async () => {
    render(
      <CreateDeploymentForm onClose={() => undefined} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test/deploy/deployments' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            ListInferenceServer: { inferenceServerList: { items: [] } },
          }),
        }),
      ])
    );

    await screen.findByRole('dialog', { name: 'Create deployment' });

    const onlineRadio = screen.getByRole('radio', { name: 'Online' });
    const offlineRadio = screen.getByRole('radio', { name: 'Offline' });

    expect(onlineRadio).toBeChecked();
    expect(onlineRadio).toBeDisabled();
    expect(offlineRadio).not.toBeChecked();
    expect(offlineRadio).toBeDisabled();
  });

  it('submits the deployment with the entered data', async () => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({
      ListInferenceServer: { inferenceServerList: { items: [{ metadata: { name: 'triton-server' } }] } },
      ListModelFamily: {
        modelFamilyList: { items: [{ metadata: { name: 'family-1' }, spec: { name: 'Family One' } }] },
      },
      ListModel: { modelList: { items: [{ metadata: { name: 'model-1' } }] } },
      CreateDeployment: {},
    });

    render(
      <CreateDeploymentForm onClose={() => undefined} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test/deploy/deployments' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    const dialog = await screen.findByRole('dialog', { name: 'Create deployment' });

    await user.type(within(dialog).getByRole('textbox', { name: 'Name *' }), 'my-deployment');
    await user.click(within(dialog).getByRole('combobox', { name: 'Inference server *' }));
    await user.click(await screen.findByText('triton-server'));

    await user.click(within(dialog).getByRole('combobox', { name: 'Model family' }));
    await user.click(await screen.findByText('Family One'));

    await user.click(within(dialog).getByRole('combobox', { name: 'Model *' }));
    await user.click(await screen.findByText('model-1'));

    await user.click(within(dialog).getByRole('button', { name: 'Create' }));

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        'CreateDeployment',
        {
          metadata: { name: 'my-deployment', namespace: 'ma-dev-test' },
          spec: {
            desiredRevision: { name: 'model-1', namespace: 'ma-dev-test' },
            target: {
              case: 'inferenceServer',
              value: { name: 'triton-server', namespace: 'ma-dev-test' },
            },
            strategy: { rolloutStrategy: { case: 'rolling', value: { incrementPercentage: 0 } } },
            definition: { type: 1 },
          },
        },
        {}
      );
    });
  });
});
