import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { CreateInferenceServerForm } from '#core/config/entities/targets/create-inference-server-form';
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

describe('CreateInferenceServerForm', () => {
  it('submits inference server with correct data structure', async () => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({
      ListCluster: {
        clusterList: {
          items: [
            {
              metadata: { name: 'michelangelo-sandbox-inference', namespace: 'ma-system' },
              spec: {
                cluster: {
                  case: 'kubernetes',
                  value: {
                    rest: {
                      host: 'https://kubernetes.default.svc',
                      port: '443',
                      tokenTag: 'cluster-michelangelo-sandbox-is-token',
                      caDataTag: 'cluster-michelangelo-sandbox-ca-data',
                    },
                  },
                },
              },
            },
          ],
        },
      },
      CreateInferenceServer: { inferenceServer: { metadata: { name: 'my-target' } } },
    });

    render(
      <CreateInferenceServerForm onClose={vi.fn()} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test/deploy/targets' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    const dialog = await screen.findByRole('dialog', { name: 'Create target' });

    await user.type(within(dialog).getByRole('textbox', { name: 'Name *' }), 'full-target');

    await user.click(within(dialog).getByLabelText('Backend', { exact: false }));
    await user.click(await screen.findByRole('option', { name: 'Dynamo' }));

    await within(dialog).findByRole('combobox', {
      name: 'Selected michelangelo-sandbox-inference.',
    });

    await user.click(within(dialog).getByLabelText('Service type', { exact: false }));
    await user.click(await screen.findByRole('option', { name: 'Triton GPU' }));

    await user.type(within(dialog).getByRole('textbox', { name: 'Server version' }), 'v1.2.3');

    const cpuField = within(dialog).getByRole('spinbutton', { name: 'CPU cores *' });
    await user.clear(cpuField);
    await user.type(cpuField, '4');

    const memoryField = within(dialog).getByRole('textbox', { name: 'Memory *' });
    await user.clear(memoryField);
    await user.type(memoryField, '8Gi');

    const gpuField = within(dialog).getByRole('spinbutton', { name: 'GPUs' });
    await user.clear(gpuField);
    await user.type(gpuField, '1');

    const replicasField = within(dialog).getByRole('spinbutton', { name: 'Replicas *' });
    await user.clear(replicasField);
    await user.type(replicasField, '3');

    await user.click(within(dialog).getByRole('button', { name: 'Create' }));

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        'CreateInferenceServer',
        expect.objectContaining({
          metadata: { name: 'full-target', namespace: 'ma-dev-test' },
          spec: {
            tenancyType: 1,
            backendType: 3,
            initSpec: {
              resourceSpec: { cpu: 4, memory: '8Gi', diskSize: '', gpu: 1 },
              servingSpec: {
                version: 'v1.2.3',
                containerBuildTemplate: 'default_triton_gpu',
              },
              numInstances: 3,
            },
            clusterTargets: [
              {
                clusterId: 'michelangelo-sandbox-inference',
                connection: {
                  case: 'kubernetes',
                  value: {
                    host: 'https://kubernetes.default.svc',
                    port: '443',
                    tokenTag: 'cluster-michelangelo-sandbox-is-token',
                    caDataTag: 'cluster-michelangelo-sandbox-ca-data',
                  },
                },
              },
            ],
          },
        }) as Record<string, unknown>,
        {}
      );
    });

    // The form-only clusterIds field never reaches the wire.
    const call = mockRequest.getCall('CreateInferenceServer');
    expect(call?.args).not.toHaveProperty('clusterIds');
  });

  it('lets the user pick from multiple registered clusters and submits both as targets', async () => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({
      ListCluster: {
        clusterList: {
          items: [
            {
              metadata: { name: 'michelangelo-sandbox-inference', namespace: 'ma-system' },
              spec: {
                cluster: {
                  case: 'kubernetes',
                  value: {
                    rest: {
                      host: 'https://kubernetes.default.svc',
                      port: '443',
                      tokenTag: 'cluster-michelangelo-sandbox-is-token',
                      caDataTag: 'cluster-michelangelo-sandbox-ca-data',
                    },
                  },
                },
              },
            },
            {
              metadata: { name: 'inference-cluster-1', namespace: 'ma-system' },
              spec: {
                cluster: {
                  case: 'kubernetes',
                  value: {
                    rest: {
                      host: 'https://k3d-inference-cluster-1-server-0',
                      port: '6443',
                      tokenTag: 'cluster-inference-cluster-1-is-token',
                      caDataTag: 'cluster-inference-cluster-1-ca-data',
                    },
                  },
                },
              },
            },
          ],
        },
      },
      CreateInferenceServer: { inferenceServer: { metadata: { name: 'my-target' } } },
    });

    render(
      <CreateInferenceServerForm onClose={vi.fn()} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test/deploy/targets' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    const dialog = await screen.findByRole('dialog', { name: 'Create target' });
    await user.type(within(dialog).getByRole('textbox', { name: 'Name *' }), 'my-target');

    // Nothing is preselected once more than one cluster is registered.
    const clusterField = within(dialog).getByRole('combobox', { name: 'Cluster targets *' });
    await user.click(clusterField);
    await user.click(await screen.findByRole('option', { name: 'inference-cluster-1' }));
    await user.click(clusterField);
    await user.click(await screen.findByRole('option', { name: 'michelangelo-sandbox-inference' }));

    await user.click(within(dialog).getByRole('button', { name: 'Create' }));

    await waitFor(() => {
      const call = mockRequest.getCall('CreateInferenceServer');
      const clusterIds = (
        call?.args as { spec: { clusterTargets: Array<{ clusterId: string }> } }
      ).spec.clusterTargets.map((target) => target.clusterId);
      expect(clusterIds).toEqual(['inference-cluster-1', 'michelangelo-sandbox-inference']);
    });
  });

  it('requires a name before the server can be created', async () => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({
      ListCluster: {
        clusterList: {
          items: [
            {
              metadata: { name: 'michelangelo-sandbox-inference', namespace: 'ma-system' },
              spec: {
                cluster: {
                  case: 'kubernetes',
                  value: { rest: { host: 'https://kubernetes.default.svc' } },
                },
              },
            },
          ],
        },
      },
    });

    render(
      <CreateInferenceServerForm onClose={vi.fn()} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test/deploy/targets' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    const dialog = await screen.findByRole('dialog', { name: 'Create target' });
    // The sole cluster is preselected, so leaving the name blank is the only invalid field.
    await within(dialog).findByRole('combobox', {
      name: 'Selected michelangelo-sandbox-inference.',
    });

    await user.click(within(dialog).getByRole('button', { name: 'Create' }));

    // FormDialog always renders a FormErrorBanner alongside each field's own caption, so a
    // page-wide text search would also match that summary. Scope to the Name field's own
    // caption via its aria-describedby to confirm the error belongs to Name specifically.
    const nameInput = within(dialog).getByRole('textbox', { name: 'Name *' });
    const nameCaptionId = nameInput.getAttribute('aria-describedby');
    await waitFor(() => {
      expect(document.getElementById(nameCaptionId ?? '')).toHaveTextContent(
        'This field is required.'
      );
    });
    expect(mockRequest).not.toHaveBeenCalledWith(
      'CreateInferenceServer',
      expect.anything(),
      expect.anything()
    );
  });

  it('requires at least one cluster target before the server can be created', async () => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({
      ListCluster: {
        clusterList: {
          items: [
            {
              metadata: { name: 'cluster-a', namespace: 'ma-system' },
              spec: {
                cluster: {
                  case: 'kubernetes',
                  value: { rest: { host: 'https://cluster-a.example.com' } },
                },
              },
            },
            {
              metadata: { name: 'cluster-b', namespace: 'ma-system' },
              spec: {
                cluster: {
                  case: 'kubernetes',
                  value: { rest: { host: 'https://cluster-b.example.com' } },
                },
              },
            },
          ],
        },
      },
    });

    render(
      <CreateInferenceServerForm onClose={vi.fn()} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test/deploy/targets' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    const dialog = await screen.findByRole('dialog', { name: 'Create target' });
    await user.type(within(dialog).getByRole('textbox', { name: 'Name *' }), 'my-target');
    await user.click(within(dialog).getByRole('button', { name: 'Create' }));

    // FormDialog always renders a FormErrorBanner alongside each field's own caption, so a
    // page-wide text search would also match that summary. Scope to the Cluster targets field's
    // own caption via its aria-describedby to confirm the error belongs to it specifically.
    const clusterField = within(dialog).getByRole('combobox', { name: 'Cluster targets *' });
    const clusterCaptionId = clusterField.getAttribute('aria-describedby');
    await waitFor(() => {
      expect(document.getElementById(clusterCaptionId ?? '')).toHaveTextContent(
        'This field is required.'
      );
    });
    expect(mockRequest).not.toHaveBeenCalledWith(
      'CreateInferenceServer',
      expect.anything(),
      expect.anything()
    );
  });
});
