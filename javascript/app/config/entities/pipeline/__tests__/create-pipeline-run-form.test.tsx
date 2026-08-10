import { useState } from 'react';
import {
  buildWrapper,
  createQueryMockRouter,
  getBaseProviderWrapper,
  getErrorProviderWrapper,
  getIconProviderWrapper,
  getInterpolationProviderWrapper,
  getRouterWrapper,
  getServiceProviderWrapper,
} from '@michelangelo-ai/core/test-utils';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { CreatePipelineRunForm } from '../create-pipeline-run-form';

describe('CreatePipelineRunForm', () => {
  // Mount-when-visible pattern: the dispatcher mounts the component while open and
  // unmounts on close. This wrapper mirrors that — unmounting on onClose.
  function FormWrapper() {
    const [mounted, setMounted] = useState(true);
    const data = {
      metadata: { name: 'test-pipeline', namespace: 'test-namespace' },
      spec: { owner: { name: 'test-owner' } },
    };
    if (!mounted) return null;
    return <CreatePipelineRunForm record={data} onClose={() => setMounted(false)} />;
  }

  it('submits pipeline run with correct data structure and closes dialog', async () => {
    const user = userEvent.setup();
    const mockResponse = { pipelineRun: { metadata: { name: 'created-run' } } };
    const mockRequest = createQueryMockRouter({
      CreatePipelineRun: mockResponse,
    });

    render(
      <FormWrapper />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test/train/pipelines' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    const dialog = await screen.findByRole('dialog', { name: 'Start new pipeline run' });
    const submitButton = within(dialog).getByRole('button', { name: 'Run' });
    await user.click(submitButton);

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        'CreatePipelineRun',
        expect.objectContaining({
          // cast: expect.objectContaining returns a matcher typed as the argument shape, not
          // Record<string, unknown>; narrowing to the mock-call shape asserted against
          metadata: expect.objectContaining({
            // cast: expect.stringMatching returns a matcher typed as unknown, not string;
            // narrowing to the mock-call shape asserted against
            name: expect.stringMatching(/^run-\d{8}-\d{6}-.+$/) as string,
            namespace: 'ma-dev-test',
          }) as Record<string, unknown>,
          // cast: expect.objectContaining returns a matcher typed as the argument shape, not
          // Record<string, unknown>; narrowing to the mock-call shape asserted against
          spec: expect.objectContaining({
            pipeline: {
              name: 'test-pipeline',
              namespace: 'ma-dev-test',
            },
          }) as Record<string, unknown>,
        }),
        {}
      );
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
      <FormWrapper />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test/train/pipelines' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    const dialog = await screen.findByRole('dialog');
    const submitButton = within(dialog).getByRole('button', { name: 'Run' });
    await user.click(submitButton);

    await screen.findByText(/Test error/);
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('includes description in payload when filled in', async () => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({ CreatePipelineRun: {} });

    render(
      <FormWrapper />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test/train/pipelines' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    const dialog = await screen.findByRole('dialog', { name: 'Start new pipeline run' });
    await user.type(
      screen.getByRole('textbox', { name: /description/i }),
      'nightly evaluation run'
    );
    await user.click(within(dialog).getByRole('button', { name: 'Run' }));

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        'CreatePipelineRun',
        expect.objectContaining({
          // cast: expect.objectContaining returns a matcher typed as the argument shape, not
          // Record<string, unknown>; narrowing to the mock-call shape asserted against
          spec: expect.objectContaining({
            description: 'nightly evaluation run',
          }) as Record<string, unknown>,
        }),
        {}
      );
    });
  });
});
