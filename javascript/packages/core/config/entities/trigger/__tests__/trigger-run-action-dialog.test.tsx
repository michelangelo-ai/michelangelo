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
import { TriggerRunActionDialog } from '../trigger-run-action-dialog';
import { TriggerRunAction, TriggerRunState } from '../types';

import type { TriggerRun } from '../types';

const buildTriggerRun = (state: TriggerRunState = TriggerRunState.RUNNING): TriggerRun => ({
  metadata: { name: 'my-trigger', namespace: 'test-ns' },
  spec: {
    pipeline: { name: 'test-pipeline', namespace: 'test-ns' },
    revision: { name: 'test-revision', namespace: 'test-ns' },
    actor: { name: 'test-user' },
  },
  status: { state },
});

const defaultWrappers = (mockRequest = createQueryMockRouter({ UpdateTriggerRun: { triggerRun: buildTriggerRun() } })) =>
  buildWrapper([
    getBaseProviderWrapper(),
    getIconProviderWrapper(),
    getErrorProviderWrapper(),
    getRouterWrapper({ location: '/test-ns/triggers' }),
    getServiceProviderWrapper({ request: mockRequest }),
  ]);

describe('TriggerRunActionDialog', () => {
  it('renders the Kill button', () => {
    render(<TriggerRunActionDialog record={buildTriggerRun()} action="kill" />, defaultWrappers());
    expect(screen.getByRole('button', { name: 'Kill' })).toBeInTheDocument();
  });

  it('renders the Pause button', () => {
    render(<TriggerRunActionDialog record={buildTriggerRun()} action="pause" />, defaultWrappers());
    expect(screen.getByRole('button', { name: 'Pause' })).toBeInTheDocument();
  });

  it('renders the Resume button', () => {
    render(<TriggerRunActionDialog record={buildTriggerRun()} action="resume" />, defaultWrappers());
    expect(screen.getByRole('button', { name: 'Resume' })).toBeInTheDocument();
  });

  it('opens dialog on button click', async () => {
    const user = userEvent.setup();
    render(<TriggerRunActionDialog record={buildTriggerRun()} action="kill" />, defaultWrappers());
    await user.click(screen.getByRole('button', { name: 'Kill' }));
    expect(await screen.findByRole('dialog')).toBeInTheDocument();
    expect(screen.getByText('Kill Trigger Run')).toBeInTheDocument();
    expect(screen.getByDisplayValue('my-trigger')).toBeInTheDocument();
  });

  it('submits kill mutation with KILL action', async () => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({ UpdateTriggerRun: { triggerRun: buildTriggerRun() } });
    render(<TriggerRunActionDialog record={buildTriggerRun()} action="kill" />, defaultWrappers(mockRequest));
    await user.click(screen.getByRole('button', { name: 'Kill' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Kill' }));
    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        'UpdateTriggerRun',
        expect.objectContaining({
          triggerRun: expect.objectContaining({
            spec: expect.objectContaining({ action: TriggerRunAction.KILL }),
          }),
        })
      );
    });
  });

  it('submits pause mutation with PAUSE action', async () => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({ UpdateTriggerRun: { triggerRun: buildTriggerRun() } });
    render(<TriggerRunActionDialog record={buildTriggerRun()} action="pause" />, defaultWrappers(mockRequest));
    await user.click(screen.getByRole('button', { name: 'Pause' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Pause' }));
    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        'UpdateTriggerRun',
        expect.objectContaining({
          triggerRun: expect.objectContaining({
            spec: expect.objectContaining({ action: TriggerRunAction.PAUSE }),
          }),
        })
      );
    });
  });

  it('submits resume mutation with RESUME action', async () => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({ UpdateTriggerRun: { triggerRun: buildTriggerRun() } });
    render(<TriggerRunActionDialog record={buildTriggerRun()} action="resume" />, defaultWrappers(mockRequest));
    await user.click(screen.getByRole('button', { name: 'Resume' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Resume' }));
    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        'UpdateTriggerRun',
        expect.objectContaining({
          triggerRun: expect.objectContaining({
            spec: expect.objectContaining({ action: TriggerRunAction.RESUME }),
          }),
        })
      );
    });
  });

  it('submits kill mutation and dialog closes on success', async () => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({ UpdateTriggerRun: { triggerRun: buildTriggerRun() } });
    render(<TriggerRunActionDialog record={buildTriggerRun()} action="kill" />, defaultWrappers(mockRequest));
    await user.click(screen.getByRole('button', { name: 'Kill' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Kill' }));
    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith('UpdateTriggerRun', expect.anything());
    });
  });
});