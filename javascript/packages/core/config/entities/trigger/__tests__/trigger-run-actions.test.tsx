import { render, screen } from '@testing-library/react';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import {
  createQueryMockRouter,
  getServiceProviderWrapper,
} from '#core/test/wrappers/get-service-provider-wrapper';
import { TriggerRunActions } from '../trigger-run-actions';
import { TriggerRunState } from '../types';

import type { TriggerRun } from '../types';

const buildTriggerRun = (state: TriggerRunState): TriggerRun => ({
  metadata: { name: 'test-trigger', namespace: 'test-ns' },
  spec: {
    pipeline: { name: 'test-pipeline', namespace: 'test-ns' },
    revision: { name: 'test-revision', namespace: 'test-ns' },
    actor: { name: 'test-user' },
  },
  status: { state },
});

const defaultWrappers = () =>
  buildWrapper([
    getBaseProviderWrapper(),
    getIconProviderWrapper(),
    getErrorProviderWrapper(),
    getRouterWrapper({ location: '/test-ns/triggers' }),
    getServiceProviderWrapper({
      request: createQueryMockRouter({ UpdateTriggerRun: { triggerRun: buildTriggerRun(TriggerRunState.KILLED) } }),
    }),
  ]);

describe('TriggerRunActions', () => {
  it('shows no buttons when state is missing', () => {
    const triggerRun = { ...buildTriggerRun(TriggerRunState.RUNNING), status: {} } as unknown as TriggerRun;
    render(<TriggerRunActions record={triggerRun} />, defaultWrappers());
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  it('shows Kill and Pause buttons when RUNNING', () => {
    const triggerRun = buildTriggerRun(TriggerRunState.RUNNING);
    render(<TriggerRunActions record={triggerRun} />, defaultWrappers());
    expect(screen.getByRole('button', { name: 'Kill' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Pause' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Resume' })).not.toBeInTheDocument();
  });

  it('shows Resume button when PAUSED', () => {
    const triggerRun = buildTriggerRun(TriggerRunState.PAUSED);
    render(<TriggerRunActions record={triggerRun} />, defaultWrappers());
    expect(screen.getByRole('button', { name: 'Resume' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Kill' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Pause' })).not.toBeInTheDocument();
  });

  it('shows no buttons when FAILED', () => {
    const triggerRun = buildTriggerRun(TriggerRunState.FAILED);
    render(<TriggerRunActions record={triggerRun} />, defaultWrappers());
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  it('shows no buttons when KILLED', () => {
    const triggerRun = buildTriggerRun(TriggerRunState.KILLED);
    render(<TriggerRunActions record={triggerRun} />, defaultWrappers());
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });
});