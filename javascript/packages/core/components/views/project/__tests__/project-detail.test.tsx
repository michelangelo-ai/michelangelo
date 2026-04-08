import { render, screen, within } from '@testing-library/react';
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

const mockNavigate = vi.fn();
vi.mock('react-router-dom-v5-compat', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router-dom-v5-compat')>();
  return { ...actual, useNavigate: () => mockNavigate };
});

describe('ProjectDetail', () => {
  const buildPhase = buildPhaseConfigFactory();
  const buildEntity = buildEntityConfigFactory();

  function renderProjectDetail() {
    const mockRequest = createQueryMockRouter({
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
    });

    const phases = [
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
      buildPhase({
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
      }),
      buildPhase({ id: 'deploy', name: 'Deploy & Predict', state: 'comingSoon' }),
    ];

    return render(
      <ProjectDetail phases={phases} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getRouterWrapper({ location: '/fraud-detection' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );
  }

  test('renders project description from API', async () => {
    renderProjectDetail();

    expect(await screen.findByText('Detects fraudulent transactions')).toBeInTheDocument();
  });

  test('renders all three phase card names', async () => {
    renderProjectDetail();

    expect(await screen.findByText('Prepare & Analyze Data')).toBeInTheDocument();
    expect(screen.getByText('Train & Evaluate')).toBeInTheDocument();
    expect(screen.getByText('Deploy & Predict')).toBeInTheDocument();
  });

  describe('disabled phase', () => {
    test('renders all entities as plain text', async () => {
      renderProjectDetail();

      await screen.findByText('Prepare & Analyze Data');

      // 'Pipelines' also appears in the active phase as a link — find the span specifically
      const pipelinesSpan = screen.getAllByText('Pipelines').find((el) => el.tagName === 'SPAN');
      expect(pipelinesSpan).toBeDefined();

      const pipelineRunsSpan = screen
        .getAllByText('Pipeline runs')
        .find((el) => el.tagName === 'SPAN');
      expect(pipelineRunsSpan).toBeDefined();

      expect(screen.getByText('Data sources').tagName).toBe('SPAN');
    });

    test('does not render a navigate button', async () => {
      renderProjectDetail();

      const heading = await screen.findByText('Prepare & Analyze Data');
      const card = heading.closest('div[class]')!.parentElement!;

      expect(within(card).queryByRole('button')).not.toBeInTheDocument();
    });
  });

  describe('active phase', () => {
    test('renders active entities as links with correct hrefs', async () => {
      renderProjectDetail();

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
    });

    test('renders disabled entities as plain text', async () => {
      renderProjectDetail();

      await screen.findByText('Train & Evaluate');

      for (const name of ['Evaluations', 'Notebooks']) {
        const el = screen.getByText(name);
        expect(el.tagName).toBe('SPAN');
        expect(el).not.toHaveRole('link');
      }
    });

    test('navigate button goes to first active entity', async () => {
      const user = userEvent.setup();
      renderProjectDetail();

      // Scope to the active phase card by walking up from a link unique to it
      const pipelinesLink = await screen.findByRole('link', { name: 'Pipelines' });
      let activeCard: HTMLElement = pipelinesLink;
      while (activeCard.parentElement && within(activeCard).queryAllByRole('button').length === 0) {
        activeCard = activeCard.parentElement;
      }

      await user.click(within(activeCard).getAllByRole('button').at(-1)!);
      expect(mockNavigate).toHaveBeenCalledWith('/fraud-detection/train/pipelines');
    });
  });

  describe('comingSoon phase', () => {
    test('shows "Coming soon" message', async () => {
      renderProjectDetail();

      expect(await screen.findByText('Coming soon')).toBeInTheDocument();
    });

    test('does not render a navigate button', async () => {
      renderProjectDetail();

      const heading = await screen.findByText('Deploy & Predict');
      const card = heading.closest('div[class]')!.parentElement!;

      expect(within(card).queryByRole('button')).not.toBeInTheDocument();
    });
  });
});
