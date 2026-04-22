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
import { KillTriggerRunForm } from '../trigger-run-action-form';
import { TriggerRunAction, TriggerRunState } from '../types';

import type { ActionComponentProps } from '#core/components/actions/types';
import type { TriggerRun } from '../types';

// Provides real isOpen state so FormDialog can close on success.
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
      sourceTriggerName: '',
      autoFlip: false,
      notifications: [],
      kill: false,
      action: TriggerRunAction.NO_ACTION,
    },
    status: { state: TriggerRunState.RUNNING },
  };
  return <Form record={record} isOpen={isOpen} onClose={() => setIsOpen(false)} />;
}

it.each([
  {
    Form: KillTriggerRunForm,
    dialogName: 'Kill Trigger Run',
    buttonLabel: 'Kill',
    action: TriggerRunAction.KILL,
  },
])(
  '$dialogName: submits UpdateTriggerRun with correct action and closes dialog',
  async ({ Form, dialogName, buttonLabel, action }) => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({
      UpdateTriggerRun: { triggerRun: { metadata: { name: 'my-trigger' } } },
    });

    render(
      <FormWrapper Form={Form} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/test-ns/triggers' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    const dialog = await screen.findByRole('dialog', { name: dialogName });
    await user.click(within(dialog).getByRole('button', { name: buttonLabel }));

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        'UpdateTriggerRun',
        expect.objectContaining({
          triggerRun: expect.objectContaining({
            spec: expect.objectContaining({ action }) as Record<string, unknown>,
          }) as Record<string, unknown>,
        })
      );
    });

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
  }
);

describe('KillTriggerRunForm', () => {
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

    await within(dialog).findByText(/Test error/);
    expect(mockRequest).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });
});
