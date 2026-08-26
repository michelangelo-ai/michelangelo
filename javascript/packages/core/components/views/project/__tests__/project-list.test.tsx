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
import { ProjectList } from '../project-list';

test('renders project names from API response', async () => {
  const mockRequest = createQueryMockRouter({
    ListProject: {
      projectList: {
        items: [
          {
            metadata: { name: 'fraud-detection' },
            spec: { description: 'Detects fraud', owner: { owningTeam: 'ml-team' }, tier: 'P0' },
          },
          {
            metadata: { name: 'recommendation-engine' },
            spec: {
              description: 'Recommends items',
              owner: { owningTeam: 'reco-team' },
              tier: 'P1',
            },
          },
        ],
      },
    },
  });

  render(
    <ProjectList />,
    buildWrapper([
      getBaseProviderWrapper(),
      getErrorProviderWrapper(),
      getIconProviderWrapper(),
      getRouterWrapper({ location: '/' }),
      getServiceProviderWrapper({ request: mockRequest }),
    ])
  );

  expect(await screen.findByText('fraud-detection')).toBeInTheDocument();
  expect(screen.getByText('recommendation-engine')).toBeInTheDocument();
});

test('resolves the Owner column to a linked display name via a registered team resolver', async () => {
  const mockRequest = createQueryMockRouter({
    ListProject: {
      projectList: {
        items: [
          {
            metadata: { name: 'fraud-detection' },
            spec: { description: 'Detects fraud', owner: { owningTeam: 'ml-team' }, tier: 'P0' },
          },
        ],
      },
    },
  });
  const resolveTeams = vi.fn().mockResolvedValue({
    'ml-team': { id: 'ml-team', displayName: 'ML Team', url: 'https://example.com/ml-team' },
  });

  render(
    <ProjectList />,
    buildWrapper([
      getBaseProviderWrapper(),
      getErrorProviderWrapper(),
      getIconProviderWrapper(),
      getRouterWrapper({ location: '/' }),
      getServiceProviderWrapper({ request: mockRequest, resolvers: { team: resolveTeams } }),
    ])
  );

  const link = await screen.findByRole('link', { name: 'ML Team' });
  expect(link).toHaveAttribute('href', 'https://example.com/ml-team');
});

test('renders column headers when no projects exist', async () => {
  const mockRequest = createQueryMockRouter({
    ListProject: { projectList: { items: [] } },
  });

  render(
    <ProjectList />,
    buildWrapper([
      getBaseProviderWrapper(),
      getErrorProviderWrapper(),
      getIconProviderWrapper(),
      getRouterWrapper({ location: '/' }),
      getServiceProviderWrapper({ request: mockRequest }),
    ])
  );

  expect(await screen.findByRole('columnheader', { name: 'Name' })).toBeInTheDocument();
  expect(screen.getByRole('columnheader', { name: 'Owner' })).toBeInTheDocument();
});
