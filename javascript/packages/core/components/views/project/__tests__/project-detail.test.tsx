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

  test('renders project description from API', async () => {
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
                  owner: { owningTeam: 'ml-team' },
                  tier: 'P0',
                },
              },
            },
          }),
        }),
      ])
    );

    expect(await screen.findByText('Detects fraudulent transactions')).toBeInTheDocument();
  });

  test('renders all three phase card names', async () => {
    render(
      <ProjectDetail
        phases={[
          buildPhase({ id: 'data', name: 'Prepare & Analyze Data', state: 'disabled', entities: [] }),
          buildPhase({ id: 'train', name: 'Train & Evaluate', state: 'active', entities: [] }),
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
                spec: {
                  description: 'Detects fraudulent transactions',
                  owner: { owningTeam: 'ml-team' },
                  tier: 'P0',
                },
              },
            },
          }),
        }),
      ])
    );

    expect(await screen.findByText('Prepare & Analyze Data')).toBeInTheDocument();
    expect(screen.getByText('Train & Evaluate')).toBeInTheDocument();
    expect(screen.getByText('Deploy & Predict')).toBeInTheDocument();
  });

  describe('disabled phase', () => {
    test('renders entities as plain text with no navigate button', async () => {
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
                  spec: {
                    description: 'Detects fraudulent transactions',
                    owner: { owningTeam: 'ml-team' },
                    tier: 'P0',
                  },
                },
              },
            }),
          }),
        ])
      );

      await screen.findByText('Prepare & Analyze Data');

      expect(screen.getByText('Pipelines').tagName).toBe('SPAN');
      expect(screen.getByText('Pipeline runs').tagName).toBe('SPAN');
      expect(screen.getByText('Data sources').tagName).toBe('SPAN');
      expect(screen.queryByRole('button')).not.toBeInTheDocument();
    });
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
                  spec: {
                    description: 'Detects fraudulent transactions',
                    owner: { owningTeam: 'ml-team' },
                    tier: 'P0',
                  },
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

      for (const name of ['Evaluations', 'Notebooks']) {
        const el = screen.getByText(name);
        expect(el.tagName).toBe('SPAN');
        expect(el).not.toHaveRole('link');
      }
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
                  spec: {
                    description: 'Detects fraudulent transactions',
                    owner: { owningTeam: 'ml-team' },
                    tier: 'P0',
                  },
                },
              },
            }),
          }),
        ])
      );

      await screen.findByText('Train & Evaluate');

      await user.click(screen.getByRole('button'));
      expect(screen.getByText(/\/fraud-detection\/train\/pipelines/)).toBeInTheDocument();
    });
  });

  describe('comingSoon phase', () => {
    test('shows "Coming soon" message with no navigate button', async () => {
      render(
        <ProjectDetail
          phases={[buildPhase({ id: 'deploy', name: 'Deploy & Predict', state: 'comingSoon' })]}
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
                  spec: {
                    description: 'Detects fraudulent transactions',
                    owner: { owningTeam: 'ml-team' },
                    tier: 'P0',
                  },
                },
              },
            }),
          }),
        ])
      );

      expect(await screen.findByText('Coming soon')).toBeInTheDocument();
      expect(screen.queryByRole('button')).not.toBeInTheDocument();
    });
  });
});
