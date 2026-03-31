import { useState } from 'react';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import {
  createQueryMockRouter,
  getServiceProviderWrapper,
} from '#core/test/wrappers/get-service-provider-wrapper';
import {
  KillTriggerRunForm,
  PauseTriggerRunForm,
  ResumeTriggerRunForm,
} from '../trigger-run-action-form';
import { TriggerRunAction, TriggerRunState } from '../types';

import type { ActionComponentProps } from '#core/components/actions/types';
import type { TriggerRun } from '../types';

// Provides real isOpen state so FormDialog can close on success.
// Record is co-located here since all three forms test the same trigger run.
function FormWrapper({
  Form,
}: {
  Form: (props: ActionComponentProps<TriggerRun>) => React.ReactElement | null;
}) {
  const [isOpen, setIsOpen] = useState(true);
  const record: TriggerRun = {
    metadata: { name: 'my-trigger', namespace: 'test-ns' },
    spec: {
      pipeline: { name: 'test-pipeline', namespace: 'test-ns' },
      revision: { name: 'test-revision', namespace: 'test-ns' },
      actor: { name: 'test-user' },
    },
    status: { state: TriggerRunState.RUNNING },
  };
  return <Form record={record} isOpen={isOpen} onClose={() => setIsOpen(false)} />;
}

describe('KillTriggerRunForm', () => {
  it('submits UpdateTriggerRun with KILL action and closes dialog on success', async () => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({
      UpdateTriggerRun: { triggerRun: { metadata: { name: 'my-trigger' } } },
    });

    render(
      <FormWrapper Form={KillTriggerRunForm} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/test-ns/triggers' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    const dialog = await screen.findByRole('dialog', { name: 'Kill Trigger Run' });
    await user.click(within(dialog).getByRole('button', { name: 'Kill' }));

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        'UpdateTriggerRun',
        expect.objectContaining({
          triggerRun: expect.objectContaining({
            spec: expect.objectContaining({ action: TriggerRunAction.KILL }) as Record<
              string,
              unknown
            >,
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
    const mockRequest = createQueryMockRouter({ UpdateTriggerRun: new Error('test') });

    render(
      <FormWrapper Form={KillTriggerRunForm} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/test-ns/triggers' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Kill' }));

    await screen.findByText(/Test error/);
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });
});

describe('PauseTriggerRunForm', () => {
  it('submits UpdateTriggerRun with PAUSE action', async () => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({
      UpdateTriggerRun: { triggerRun: { metadata: { name: 'my-trigger' } } },
    });

    render(
      <FormWrapper Form={PauseTriggerRunForm} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/test-ns/triggers' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    const dialog = await screen.findByRole('dialog', { name: 'Pause Trigger Run' });
    await user.click(within(dialog).getByRole('button', { name: 'Pause' }));

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        'UpdateTriggerRun',
        expect.objectContaining({
          triggerRun: expect.objectContaining({
            spec: expect.objectContaining({ action: TriggerRunAction.PAUSE }) as Record<
              string,
              unknown
            >,
          }) as Record<string, unknown>,
        })
      );
    });
  });
});

describe('ResumeTriggerRunForm', () => {
  it('submits UpdateTriggerRun with RESUME action', async () => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({
      UpdateTriggerRun: { triggerRun: { metadata: { name: 'my-trigger' } } },
    });

    render(
      <FormWrapper Form={ResumeTriggerRunForm} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/test-ns/triggers' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    const dialog = await screen.findByRole('dialog', { name: 'Resume Trigger Run' });
    await user.click(within(dialog).getByRole('button', { name: 'Resume' }));

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        'UpdateTriggerRun',
        expect.objectContaining({
          triggerRun: expect.objectContaining({
            spec: expect.objectContaining({ action: TriggerRunAction.RESUME }) as Record<
              string,
              unknown
            >,
          }) as Record<string, unknown>,
        })
      );
    });
  });
});
