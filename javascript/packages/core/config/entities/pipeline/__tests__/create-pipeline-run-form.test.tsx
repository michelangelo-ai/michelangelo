import { useState } from 'react';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { CreatePipelineRunForm } from '#core/config/entities/pipeline/create-pipeline-run-form';
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
        })
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
          spec: expect.objectContaining({
            description: 'nightly evaluation run',
          }) as Record<string, unknown>,
        })
      );
    });
  });

  it('notification fields are disabled until the toggle is switched on, then submit includes them', async () => {
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

    await screen.findByRole('dialog', { name: 'Start new pipeline run' });

    // Toggle is visible and off by default; email input is present but disabled.
    // The toggle's accessible name is its own label text ("Enabled"/"Disabled"), which
    // starts as "Disabled" since notifications default to off.
    expect(screen.getByText('Send notifications')).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: /email addresses/i })).toBeDisabled();

    // Enable notifications via toggle
    await user.click(screen.getByRole('checkbox', { name: 'Disabled' }));

    // Email input is now enabled
    const emailInput = await screen.findByRole('textbox', { name: /email addresses/i });
    await waitFor(() => {
      expect(emailInput).toBeEnabled();
    });

    // Type an email and submit — payload should include the notification
    await user.type(emailInput, 'notify@example.com');
    await user.keyboard('{Enter}');
    await user.click(screen.getByRole('button', { name: 'Run' }));

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        'CreatePipelineRun',
        expect.objectContaining({
          spec: expect.objectContaining({
            notifications: [
              expect.objectContaining({
                emails: ['notify@example.com'],
                slackDestinations: [],
                // Default selection covers every trigger condition (see NOTIFICATION_EVENT_TYPES).
                eventTypes: [11, 1, 2, 3, 4],
              }),
            ],
          }) as Record<string, unknown>,
        })
      );
    });
  });

  it('shows a validation error when an invalid email address is entered', async () => {
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

    await screen.findByRole('dialog', { name: 'Start new pipeline run' });

    // Enable notifications
    await user.click(screen.getByRole('checkbox', { name: 'Disabled' }));

    // Type an invalid email into the email addresses input and commit with Enter
    const emailInput = await screen.findByRole('textbox', { name: /email addresses/i });
    await user.type(emailInput, 'not-an-email');
    await user.keyboard('{Enter}');

    await screen.findAllByText('Enter valid email addresses, e.g. user@example.com.');

    // Submitting with the invalid email must not call the API or close the dialog.
    await user.click(screen.getByRole('button', { name: 'Run' }));

    await screen.findAllByText('Enter valid email addresses, e.g. user@example.com.');
    expect(mockRequest).not.toHaveBeenCalled();
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });
});
