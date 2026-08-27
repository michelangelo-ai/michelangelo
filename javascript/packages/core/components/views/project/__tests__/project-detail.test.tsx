import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import {
  buildEntityConfigFactory,
  buildPhaseConfigFactory,
} from '#core/router/__fixtures__/phase-config-factory';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import {
  createQueryMockRouter,
  getServiceProviderWrapper,
} from '#core/test/wrappers/get-service-provider-wrapper';
import { ProjectDetail } from '../project-detail';

describe('ProjectDetail', () => {
  const buildPhase = buildPhaseConfigFactory();
  const buildEntity = buildEntityConfigFactory();

  test('renders project name and description from API', async () => {
    render(
      <ProjectDetail phases={[]} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getRouterWrapper({ location: '/fraud-detection' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            GetProject: {
              project: {
                metadata: { name: 'fraud-detection' },
                spec: { description: 'Detects fraudulent transactions' },
              },
            },
          }),
        }),
      ])
    );

    expect(await screen.findByText('Detects fraudulent transactions')).toBeInTheDocument();
    expect(screen.getAllByText('fraud-detection')).not.toHaveLength(0);
  });

  test('resolves the Owner field to a linked display name via a registered team resolver', async () => {
    const resolveTeams = vi.fn().mockResolvedValue({
      'uuid-1': { id: 'uuid-1', displayName: 'Michelangelo', url: 'https://example.com/team' },
    });

    render(
      <ProjectDetail phases={[]} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getRouterWrapper({ location: '/fraud-detection' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            GetProject: {
              project: {
                metadata: { name: 'fraud-detection' },
                spec: {
                  description: 'Detects fraudulent transactions',
                  owner: { owningTeam: 'uuid-1' },
                },
              },
            },
          }),
          resolvers: { team: resolveTeams },
        }),
      ])
    );

    const link = await screen.findByRole('link', { name: 'Michelangelo' });
    expect(link).toHaveAttribute('href', 'https://example.com/team');
    expect(resolveTeams).toHaveBeenCalledWith(['uuid-1']);
  });

  test('falls back to the raw owningTeam UUID as plain text when unenriched', async () => {
    render(
      <ProjectDetail phases={[]} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getRouterWrapper({ location: '/fraud-detection' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            GetProject: {
              project: {
                metadata: { name: 'fraud-detection' },
                spec: {
                  description: 'Detects fraudulent transactions',
                  owner: { owningTeam: 'uuid-1' },
                },
              },
            },
          }),
        }),
      ])
    );

    expect(await screen.findByText('uuid-1')).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'uuid-1' })).not.toBeInTheDocument();
  });

  test('renders a source code link when gitRepo is set', async () => {
    render(
      <ProjectDetail phases={[]} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getRouterWrapper({ location: '/fraud-detection' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            GetProject: {
              project: {
                metadata: { name: 'fraud-detection' },
                spec: {
                  description: 'Detects fraudulent transactions',
                  gitRepo: 'https://github.com/example-org/fraud-detection',
                },
              },
            },
          }),
        }),
      ])
    );

    const link = await screen.findByRole('link', { name: 'Link' });
    expect(link).toHaveAttribute('href', 'https://github.com/example-org/fraud-detection');
  });

  test('omits the source code link when gitRepo is not set', async () => {
    render(
      <ProjectDetail phases={[]} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getRouterWrapper({ location: '/fraud-detection' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            GetProject: {
              project: {
                metadata: { name: 'fraud-detection' },
                spec: { description: 'Detects fraudulent transactions' },
              },
            },
          }),
        }),
      ])
    );

    await screen.findByText('Detects fraudulent transactions');
    expect(screen.queryByText('Source Code')).not.toBeInTheDocument();
  });

  test('renders all three phase cards with correct states', async () => {
    render(
      <ProjectDetail
        phases={[
          buildPhase({
            id: 'data',
            name: 'Prepare & Analyze Data',
            state: 'disabled',
            entities: [],
          }),
          buildPhase({
            id: 'train',
            name: 'Train & Evaluate',
            state: 'active',
            entities: [buildEntity({ id: 'pipelines', name: 'pipelines' })],
          }),
          buildPhase({ id: 'deploy', name: 'Deploy & Predict', state: 'comingSoon' }),
        ]}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getRouterWrapper({ location: '/fraud-detection' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            GetProject: {
              project: {
                metadata: { name: 'fraud-detection' },
                spec: { description: 'Detects fraudulent transactions' },
              },
            },
          }),
        }),
      ])
    );

    expect(await screen.findByText('Prepare & Analyze Data')).toBeInTheDocument();
    expect(screen.getByText('Train & Evaluate')).toBeInTheDocument();
    expect(screen.getByText('Deploy & Predict')).toBeInTheDocument();

    expect(screen.getByRole('link', { name: 'pipelines' })).toHaveAttribute(
      'href',
      '/fraud-detection/train/pipelines'
    );
    expect(screen.getByText('Coming soon')).toBeInTheDocument();
  });

  test('disabled phase renders entities as plain text with no navigate button', async () => {
    render(
      <ProjectDetail
        phases={[
          buildPhase({
            id: 'data',
            name: 'Prepare & Analyze Data',
            state: 'disabled',
            entities: [
              buildEntity({ id: 'pipelines', name: 'pipelines', state: 'disabled' }),
              buildEntity({ id: 'runs', name: 'pipeline runs', state: 'disabled' }),
              buildEntity({ id: 'datasources', name: 'data sources', state: 'disabled' }),
            ],
          }),
        ]}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getRouterWrapper({ location: '/fraud-detection' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            GetProject: {
              project: {
                metadata: { name: 'fraud-detection' },
                spec: { description: 'Detects fraudulent transactions' },
              },
            },
          }),
        }),
      ])
    );

    await screen.findByText('Prepare & Analyze Data');

    expect(screen.getByText('pipelines')).toBeInTheDocument();
    expect(screen.getByText('pipeline runs')).toBeInTheDocument();
    expect(screen.getByText('data sources')).toBeInTheDocument();

    expect(screen.queryByRole('link', { name: 'pipelines' })).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'pipeline runs' })).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'data sources' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  test('comingSoon phase shows a "Coming soon" badge and disables its entity list', async () => {
    render(
      <ProjectDetail
        phases={[
          buildPhase({
            id: 'deploy',
            name: 'Deploy & Predict',
            state: 'comingSoon',
            entities: [buildEntity({ id: 'endpoints', name: 'endpoints' })],
          }),
        ]}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getRouterWrapper({ location: '/fraud-detection' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            GetProject: {
              project: {
                metadata: { name: 'fraud-detection' },
                spec: { description: 'Detects fraudulent transactions' },
              },
            },
          }),
        }),
      ])
    );

    await screen.findByText('Deploy & Predict');

    expect(screen.getByText('Coming soon')).toBeInTheDocument();
    expect(screen.getByText('endpoints')).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'endpoints' })).not.toBeInTheDocument();
  });

  test('phase description and learn more button render when docUrl is set', async () => {
    render(
      <ProjectDetail
        phases={[
          buildPhase({
            id: 'train',
            name: 'Train & Evaluate',
            state: 'active',
            description: 'Train your ML models',
            docUrl: 'https://docs.example.com/train',
            entities: [],
          }),
        ]}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getRouterWrapper({ location: '/fraud-detection' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            GetProject: {
              project: {
                metadata: { name: 'fraud-detection' },
                spec: { description: 'Detects fraudulent transactions' },
              },
            },
          }),
        }),
      ])
    );

    expect(await screen.findByText('Train your ML models')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Learn more' })).toBeInTheDocument();
  });

  describe('active phase', () => {
    const phase = buildPhase({
      id: 'train',
      name: 'Train & Evaluate',
      state: 'active',
      entities: [
        buildEntity({ id: 'pipelines', name: 'pipelines' }),
        buildEntity({ id: 'runs', name: 'pipeline runs' }),
        buildEntity({ id: 'triggers', name: 'triggers' }),
        buildEntity({ id: 'models', name: 'trained models' }),
        buildEntity({ id: 'evaluations', name: 'evaluations', state: 'disabled' }),
        buildEntity({ id: 'notebooks', name: 'notebooks', state: 'disabled' }),
      ],
    });

    test('renders active entities as links and disabled entities as plain text', async () => {
      render(
        <ProjectDetail phases={[phase]} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getErrorProviderWrapper(),
          getIconProviderWrapper(),
          getRouterWrapper({ location: '/fraud-detection' }),
          getServiceProviderWrapper({
            request: createQueryMockRouter({
              GetProject: {
                project: {
                  metadata: { name: 'fraud-detection' },
                  spec: { description: 'Detects fraudulent transactions' },
                },
              },
            }),
          }),
        ])
      );

      const links: [string, string][] = [
        ['pipelines', '/fraud-detection/train/pipelines'],
        ['pipeline runs', '/fraud-detection/train/runs'],
        ['triggers', '/fraud-detection/train/triggers'],
        ['trained models', '/fraud-detection/train/models'],
      ];

      for (const [name, href] of links) {
        const link = await screen.findByRole('link', { name });
        expect(link).toHaveAttribute('href', href);
      }

      expect(screen.getByText('evaluations')).toBeInTheDocument();
      expect(screen.getByText('notebooks')).toBeInTheDocument();
      expect(screen.queryByRole('link', { name: 'evaluations' })).not.toBeInTheDocument();
      expect(screen.queryByRole('link', { name: 'notebooks' })).not.toBeInTheDocument();
    });

    test('navigate button goes to first active entity', async () => {
      const user = userEvent.setup();
      render(
        <ProjectDetail phases={[phase]} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getErrorProviderWrapper(),
          getIconProviderWrapper(),
          getRouterWrapper({ location: '/fraud-detection' }),
          getServiceProviderWrapper({
            request: createQueryMockRouter({
              GetProject: {
                project: {
                  metadata: { name: 'fraud-detection' },
                  spec: { description: 'Detects fraudulent transactions' },
                },
              },
            }),
          }),
        ])
      );

      await screen.findByText('Train & Evaluate');

      await user.click(screen.getByRole('button', { name: 'Go to Train & Evaluate' }));
      expect(screen.getByText(/\/fraud-detection\/train\/pipelines/)).toBeInTheDocument();
    });

    test('navigate button is hidden when no entities are active', async () => {
      render(
        <ProjectDetail
          phases={[
            buildPhase({
              id: 'train',
              name: 'Train & Evaluate',
              state: 'active',
              entities: [
                buildEntity({ id: 'pipelines', name: 'pipelines', state: 'disabled' }),
                buildEntity({ id: 'runs', name: 'pipeline runs', state: 'disabled' }),
              ],
            }),
          ]}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getErrorProviderWrapper(),
          getIconProviderWrapper(),
          getRouterWrapper({ location: '/fraud-detection' }),
          getServiceProviderWrapper({
            request: createQueryMockRouter({
              GetProject: {
                project: {
                  metadata: { name: 'fraud-detection' },
                  spec: { description: 'Detects fraudulent transactions' },
                },
              },
            }),
          }),
        ])
      );

      await screen.findByText('Train & Evaluate');

      expect(screen.queryByRole('button')).not.toBeInTheDocument();
    });
  });
});
