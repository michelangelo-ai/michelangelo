import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { CreatePipelineRunDialog } from '#core/config/entities/pipeline/create-pipeline-run-dialog';
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

import type { Pipeline } from '#core/config/entities/pipeline/types';

describe('CreatePipelineRunDialog', () => {
  const mockPipeline: Pipeline = {
    metadata: {
      name: 'test-pipeline',
      namespace: 'test-namespace',
    },
    spec: {
      owner: {
        name: 'test-owner',
      },
    },
  };

  it('submits pipeline run with correct data structure and closes dialog', async () => {
    const user = userEvent.setup();
    const mockResponse = { pipelineRun: { metadata: { name: 'created-run' } } };
    const mockRequest = createQueryMockRouter({
      CreatePipelineRun: mockResponse,
    });

    render(
      <CreatePipelineRunDialog record={mockPipeline} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test/train/pipelines' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    await user.click(screen.getByRole('button', { name: 'Run' }));
    const dialog = await screen.findByRole('dialog', { name: 'Start new pipeline run' });
    const submitButton = within(dialog).getByRole('button', { name: 'Run' });
    await user.click(submitButton);

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith('CreatePipelineRun', {
        pipelineRun: expect.objectContaining({
          metadata: expect.objectContaining({
            name: expect.stringMatching(/^run-\d{8}-\d{6}-.+$/) as string,
            namespace: 'ma-dev-test',
          }) as Record<string, unknown>,
          spec: expect.objectContaining({
            actor: {
              name: 'mastudio-user',
            },
            pipeline: {
              name: 'test-pipeline',
              namespace: 'ma-dev-test',
            },
          }) as Record<string, unknown>,
        }) as Record<string, unknown>,
      });
    });

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
  });

  it('keeps dialog open and displays error when submission fails', async () => {
    const user = userEvent.setup();
    const mockError = new Error('Test error');
    const mockRequest = createQueryMockRouter({
      CreatePipelineRun: mockError,
    });

    render(
      <CreatePipelineRunDialog record={mockPipeline} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test/train/pipelines' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    await user.click(screen.getByRole('button', { name: 'Run' }));
    const dialog = await screen.findByRole('dialog');
    const submitButton = within(dialog).getByRole('button', { name: 'Run' });
    await user.click(submitButton);

    await screen.findByText(/Test error/);
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });
});
