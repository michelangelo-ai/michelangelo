import { render, screen } from '@testing-library/react';
import { vi } from 'vitest';

import { DEPLOY_PHASE } from '#core/config/phases/deploy';
import { PhaseListRoute } from '#core/router/phase-list-route';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { getServiceProviderWrapper } from '#core/test/wrappers/get-service-provider-wrapper';

describe('Target list page', () => {
  it('renders the Targets tab', () => {
    render(
      <PhaseListRoute phases={{ deploy: DEPLOY_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/deploy/targets' }),
        getServiceProviderWrapper({
          request: vi.fn().mockResolvedValue({ inferenceServerList: { items: [] } }),
        }),
      ])
    );

    expect(screen.getByRole('tab', { name: 'Targets' })).toBeInTheDocument();
  });

  it('renders the correct column headers', async () => {
    render(
      <PhaseListRoute phases={{ deploy: DEPLOY_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/deploy/targets' }),
        getServiceProviderWrapper({
          request: vi.fn().mockResolvedValue({ inferenceServerList: { items: [] } }),
        }),
      ])
    );

    expect(await screen.findByRole('columnheader', { name: 'Target name' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Last updated' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Type' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'State' })).toBeInTheDocument();
  });
});
