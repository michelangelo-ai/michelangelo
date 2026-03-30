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
import { TriggerRunState } from '#core/config/entities/trigger/types';
import { TriggerActionCell, KillTriggerCell, PauseTriggerCell, ResumeTriggerCell } from '../trigger-action-cell';

import type { TriggerRun } from '#core/config/entities/trigger/types';

const buildTriggerRun = (state: TriggerRunState): TriggerRun => ({
  metadata: { name: 'test-trigger', namespace: 'test-ns' },
  spec: {
    pipeline: { name: 'test-pipeline', namespace: 'test-ns' },
    revision: { name: 'test-revision', namespace: 'test-ns' },
    actor: { name: 'test-user' },
  },
  status: { state },
});

const defaultWrappers = (mockRequest = createQueryMockRouter({ UpdateTriggerRun: { triggerRun: buildTriggerRun(TriggerRunState.KILLED) } })) =>
  buildWrapper([
    getBaseProviderWrapper(),
    getIconProviderWrapper(),
    getErrorProviderWrapper(),
    getRouterWrapper({ location: '/test-ns/triggers' }),
    getServiceProviderWrapper({ request: mockRequest }),
  ]);

describe('TriggerActionCell', () => {
  it('returns null when status state is missing', () => {
    const triggerRun = { ...buildTriggerRun(TriggerRunState.RUNNING), status: {} } as unknown as TriggerRun;
    render(
      <TriggerActionCell value={triggerRun} action="kill" record={triggerRun} column={{ id: 'kill' }} />,
      defaultWrappers()
    );
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  it('returns null for kill action when state is PAUSED', () => {
    const triggerRun = buildTriggerRun(TriggerRunState.PAUSED);
    render(
      <TriggerActionCell value={triggerRun} action="kill" record={triggerRun} column={{ id: 'kill' }} />,
      defaultWrappers()
    );
    expect(screen.queryByRole('button', { name: 'Kill' })).not.toBeInTheDocument();
  });

  it('returns null for resume action when state is RUNNING', () => {
    const triggerRun = buildTriggerRun(TriggerRunState.RUNNING);
    render(
      <TriggerActionCell value={triggerRun} action="resume" record={triggerRun} column={{ id: 'resume' }} />,
      defaultWrappers()
    );
    expect(screen.queryByRole('button', { name: 'Resume' })).not.toBeInTheDocument();
  });

  it('renders Kill button for RUNNING state', () => {
    const triggerRun = buildTriggerRun(TriggerRunState.RUNNING);
    render(
      <TriggerActionCell value={triggerRun} action="kill" record={triggerRun} column={{ id: 'kill' }} />,
      defaultWrappers()
    );
    expect(screen.getByRole('button', { name: 'Kill' })).toBeInTheDocument();
  });

  it('renders Pause button for RUNNING state', () => {
    const triggerRun = buildTriggerRun(TriggerRunState.RUNNING);
    render(
      <TriggerActionCell value={triggerRun} action="pause" record={triggerRun} column={{ id: 'pause' }} />,
      defaultWrappers()
    );
    expect(screen.getByRole('button', { name: 'Pause' })).toBeInTheDocument();
  });

  it('renders Resume button for PAUSED state', () => {
    const triggerRun = buildTriggerRun(TriggerRunState.PAUSED);
    render(
      <TriggerActionCell value={triggerRun} action="resume" record={triggerRun} column={{ id: 'resume' }} />,
      defaultWrappers()
    );
    expect(screen.getByRole('button', { name: 'Resume' })).toBeInTheDocument();
  });

  it('opens dialog on button click', async () => {
    const user = userEvent.setup();
    const triggerRun = buildTriggerRun(TriggerRunState.RUNNING);
    render(
      <TriggerActionCell value={triggerRun} action="kill" record={triggerRun} column={{ id: 'kill' }} />,
      defaultWrappers()
    );
    await user.click(screen.getByRole('button', { name: 'Kill' }));
    expect(await screen.findByRole('dialog')).toBeInTheDocument();
  });

  it('closes dialog on cancel', async () => {
    const user = userEvent.setup();
    const triggerRun = buildTriggerRun(TriggerRunState.RUNNING);
    render(
      <TriggerActionCell value={triggerRun} action="kill" record={triggerRun} column={{ id: 'kill' }} />,
      defaultWrappers()
    );
    await user.click(screen.getByRole('button', { name: 'Kill' }));
    await screen.findByRole('dialog');
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
  });

  it('submits kill action on confirm', async () => {
    const user = userEvent.setup();
    const triggerRun = buildTriggerRun(TriggerRunState.RUNNING);
    const mockRequest = createQueryMockRouter({
      UpdateTriggerRun: { triggerRun: buildTriggerRun(TriggerRunState.KILLED) },
    });
    render(
      <TriggerActionCell value={triggerRun} action="kill" record={triggerRun} column={{ id: 'kill' }} />,
      defaultWrappers(mockRequest)
    );
    await user.click(screen.getByRole('button', { name: 'Kill' }));
    await screen.findByRole('dialog');
    const dialog = screen.getByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Kill' }));
    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith('UpdateTriggerRun', expect.anything());
    });
  });
});

describe('KillTriggerCell / PauseTriggerCell / ResumeTriggerCell', () => {
  it('KillTriggerCell renders for RUNNING state', () => {
    const triggerRun = buildTriggerRun(TriggerRunState.RUNNING);
    render(
      <KillTriggerCell value={triggerRun} record={triggerRun} column={{ id: 'kill' }} />,
      defaultWrappers()
    );
    expect(screen.getByRole('button', { name: 'Kill' })).toBeInTheDocument();
  });

  it('PauseTriggerCell renders for RUNNING state', () => {
    const triggerRun = buildTriggerRun(TriggerRunState.RUNNING);
    render(
      <PauseTriggerCell value={triggerRun} record={triggerRun} column={{ id: 'pause' }} />,
      defaultWrappers()
    );
    expect(screen.getByRole('button', { name: 'Pause' })).toBeInTheDocument();
  });

  it('ResumeTriggerCell renders for PAUSED state', () => {
    const triggerRun = buildTriggerRun(TriggerRunState.PAUSED);
    render(
      <ResumeTriggerCell value={triggerRun} record={triggerRun} column={{ id: 'resume' }} />,
      defaultWrappers()
    );
    expect(screen.getByRole('button', { name: 'Resume' })).toBeInTheDocument();
  });
});