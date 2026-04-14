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

    expect(screen.getByRole('link', { name: 'Pipelines' })).toHaveAttribute(
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

    expect(screen.getByText('Pipelines')).toBeInTheDocument();
    expect(screen.getByText('Pipeline runs')).toBeInTheDocument();
    expect(screen.getByText('Data sources')).toBeInTheDocument();

    expect(screen.queryByRole('link', { name: 'Pipelines' })).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Pipeline runs' })).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Data sources' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  test('comingSoon phase shows message and suppresses entity list', async () => {
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
    expect(screen.queryByText('Endpoints')).not.toBeInTheDocument();
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
        ['Pipelines', '/fraud-detection/train/pipelines'],
        ['Pipeline runs', '/fraud-detection/train/runs'],
        ['Triggers', '/fraud-detection/train/triggers'],
        ['Trained models', '/fraud-detection/train/models'],
      ];

      for (const [name, href] of links) {
        const link = await screen.findByRole('link', { name });
        expect(link).toHaveAttribute('href', href);
      }

      expect(screen.getByText('Evaluations')).toBeInTheDocument();
      expect(screen.getByText('Notebooks')).toBeInTheDocument();
      expect(screen.queryByRole('link', { name: 'Evaluations' })).not.toBeInTheDocument();
      expect(screen.queryByRole('link', { name: 'Notebooks' })).not.toBeInTheDocument();
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
