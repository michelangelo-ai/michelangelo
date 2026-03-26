import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { BreadcrumbBar } from '../breadcrumb-bar';

describe('BreadcrumbBar — project page', () => {
  beforeEach(() => {
    render(
      <BreadcrumbBar
        categories={[
          {
            id: 'core-ml',
            name: 'Core ML',
            phases: [
              {
                id: 'train',
                name: 'Train & Evaluate',
                icon: '',
                state: 'active',
                entities: [
                  {
                    id: 'models',
                    name: 'trained models',
                    state: 'active',
                    service: 'model',
                    views: [],
                  },
                ],
              },
            ],
          },
        ]}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getRouterWrapper({ location: '/my-project' }),
      ])
    );
  });

  it('renders a Home link to the root', () => {
    expect(screen.getByRole('link', { name: 'Home' })).toHaveAttribute('href', '/');
  });

  it('renders the project ID as plain text (not a link)', () => {
    expect(screen.getByText('my-project')).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'my-project' })).not.toBeInTheDocument();
  });

  it('does not render category, phase, or entity breadcrumbs', () => {
    expect(screen.queryByText('Core ML')).not.toBeInTheDocument();
    expect(screen.queryByText('Train & Evaluate')).not.toBeInTheDocument();
    expect(screen.queryByText('trained models')).not.toBeInTheDocument();
  });
});

describe('BreadcrumbBar — phase entity page', () => {
  beforeEach(() => {
    render(
      <BreadcrumbBar
        categories={[
          {
            id: 'core-ml',
            name: 'Core ML',
            phases: [
              {
                id: 'train',
                name: 'Train & Evaluate',
                icon: '',
                state: 'active',
                entities: [
                  {
                    id: 'models',
                    name: 'trained models',
                    state: 'active',
                    service: 'model',
                    views: [],
                  },
                ],
              },
            ],
          },
        ]}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getRouterWrapper({ location: '/my-project/train/models' }),
      ])
    );
  });

  it('renders the project ID as a link', () => {
    expect(screen.getByRole('link', { name: 'my-project' })).toHaveAttribute('href', '/my-project');
  });

  it('renders the category name as a link to the project', () => {
    expect(screen.getByRole('link', { name: 'Core ML' })).toHaveAttribute('href', '/my-project');
  });

  it('renders the phase display name as a link, not the phase ID', () => {
    expect(screen.getByRole('link', { name: 'Train & Evaluate' })).toHaveAttribute(
      'href',
      '/my-project/train'
    );
    expect(screen.queryByText('train')).not.toBeInTheDocument();
  });

  it('renders the entity name as plain text when there is no entity ID', () => {
    expect(screen.getByText('trained models')).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'trained models' })).not.toBeInTheDocument();
  });
});

describe('BreadcrumbBar — entity detail page', () => {
  beforeEach(() => {
    render(
      <BreadcrumbBar
        categories={[
          {
            id: 'core-ml',
            name: 'Core ML',
            phases: [
              {
                id: 'train',
                name: 'Train & Evaluate',
                icon: '',
                state: 'active',
                entities: [
                  {
                    id: 'models',
                    name: 'trained models',
                    state: 'active',
                    service: 'model',
                    views: [],
                  },
                ],
              },
            ],
          },
        ]}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getRouterWrapper({ location: '/my-project/train/models/model-123' }),
      ])
    );
  });

  it('renders the entity name as a link', () => {
    expect(screen.getByRole('link', { name: 'trained models' })).toHaveAttribute(
      'href',
      '/my-project/train/models'
    );
  });

  it('renders the entity ID as plain text (not a link)', () => {
    expect(screen.getByText('model-123')).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'model-123' })).not.toBeInTheDocument();
  });
});

describe('BreadcrumbBar — unknown phase/entity fallback', () => {
  it('renders the raw phase ID when the phase is not found in config', () => {
    render(
      <BreadcrumbBar
        categories={[
          {
            id: 'core-ml',
            name: 'Core ML',
            phases: [
              { id: 'train', name: 'Train & Evaluate', icon: '', state: 'active', entities: [] },
            ],
          },
        ]}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getRouterWrapper({ location: '/my-project/unknown-phase/some-entity' }),
      ])
    );
    expect(screen.getByText('unknown-phase')).toBeInTheDocument();
  });

  it('renders the entity URL segment when the entity is not found in config', () => {
    // normalizeEntityParam calls pluralize() on the entity param; use a segment
    // that is already plural so it passes through unchanged.
    render(
      <BreadcrumbBar
        categories={[
          {
            id: 'core-ml',
            name: 'Core ML',
            phases: [
              { id: 'train', name: 'Train & Evaluate', icon: '', state: 'active', entities: [] },
            ],
          },
        ]}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getRouterWrapper({ location: '/my-project/train/unknown-models' }),
      ])
    );
    expect(screen.getByText('unknown-models')).toBeInTheDocument();
  });
});

describe('BreadcrumbBar — menu drawer', () => {
  it('renders the Menu button', () => {
    render(
      <BreadcrumbBar
        categories={[
          {
            id: 'core-ml',
            name: 'Core ML',
            phases: [
              { id: 'train', name: 'Train & Evaluate', icon: '', state: 'active', entities: [] },
            ],
          },
        ]}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getRouterWrapper({ location: '/my-project' }),
      ])
    );
    expect(screen.getByRole('button', { name: /menu/i })).toBeInTheDocument();
  });

  it('opens the drawer and shows the phase and entities', async () => {
    render(
      <BreadcrumbBar
        categories={[
          {
            id: 'core-ml',
            name: 'Core ML',
            phases: [
              {
                id: 'train',
                name: 'Train & Evaluate',
                icon: '',
                state: 'active',
                entities: [
                  {
                    id: 'models',
                    name: 'trained models',
                    state: 'active',
                    service: 'model',
                    views: [],
                  },
                ],
              },
            ],
          },
        ]}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getRouterWrapper({ location: '/my-project' }),
      ])
    );

    await userEvent.click(screen.getByRole('button', { name: /menu/i }));

    expect(screen.getByText('Train & Evaluate')).toBeInTheDocument();
    expect(screen.getByText('Trained models')).toBeInTheDocument();
  });

  it('navigates to the entity route when an active entity is clicked', async () => {
    render(
      <BreadcrumbBar
        categories={[
          {
            id: 'core-ml',
            name: 'Core ML',
            phases: [
              {
                id: 'train',
                name: 'Train & Evaluate',
                icon: '',
                state: 'active',
                entities: [
                  {
                    id: 'models',
                    name: 'trained models',
                    state: 'active',
                    service: 'model',
                    views: [],
                  },
                ],
              },
            ],
          },
        ]}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getRouterWrapper({ location: '/my-project' }),
      ])
    );

    await userEvent.click(screen.getByRole('button', { name: /menu/i }));
    await userEvent.click(screen.getByText('Trained models'));

    expect(screen.getByText(/\/my-project\/train\/models/)).toBeInTheDocument();
  });

  it('does not navigate when a disabled entity is clicked', async () => {
    render(
      <BreadcrumbBar
        categories={[
          {
            id: 'core-ml',
            name: 'Core ML',
            phases: [
              {
                id: 'train',
                name: 'Train & Evaluate',
                icon: '',
                state: 'active',
                entities: [
                  {
                    id: 'pipelines',
                    name: 'pipelines',
                    state: 'disabled',
                    service: 'pipeline',
                    views: [],
                  },
                ],
              },
            ],
          },
        ]}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getRouterWrapper({ location: '/my-project' }),
      ])
    );

    await userEvent.click(screen.getByRole('button', { name: /menu/i }));
    await userEvent.click(screen.getByText('Pipelines'));

    expect(screen.queryByText(/\/my-project\/train/)).not.toBeInTheDocument();
  });

  it('shows "Coming soon" tag for coming-soon phases', async () => {
    render(
      <BreadcrumbBar
        categories={[
          {
            id: 'core-ml',
            name: 'Core ML',
            phases: [
              { id: 'retrain', name: 'Retrain', icon: '', state: 'comingSoon', entities: [] },
            ],
          },
        ]}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getRouterWrapper({ location: '/my-project' }),
      ])
    );

    await userEvent.click(screen.getByRole('button', { name: /menu/i }));

    expect(screen.getByText('Coming soon')).toBeInTheDocument();
  });

  it('renders top-level links with correct hrefs in the drawer', async () => {
    render(
      <BreadcrumbBar
        categories={[
          {
            id: 'core-ml',
            name: 'Core ML',
            phases: [
              { id: 'train', name: 'Train & Evaluate', icon: '', state: 'active', entities: [] },
            ],
          },
        ]}
        topLevelLinks={[
          { label: 'Settings', path: '/settings' },
          { label: 'Dashboard', path: '/dashboard' },
        ]}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getRouterWrapper({ location: '/my-project' }),
      ])
    );

    await userEvent.click(screen.getByRole('button', { name: /menu/i }));

    expect(screen.getByRole('link', { name: /settings/i })).toHaveAttribute('href', '/settings');
    expect(screen.getByRole('link', { name: /dashboard/i })).toHaveAttribute('href', '/dashboard');
  });

  it('navigates when a top-level link is clicked', async () => {
    render(
      <BreadcrumbBar
        categories={[
          {
            id: 'core-ml',
            name: 'Core ML',
            phases: [
              { id: 'train', name: 'Train & Evaluate', icon: '', state: 'active', entities: [] },
            ],
          },
        ]}
        topLevelLinks={[{ label: 'Settings', path: '/settings' }]}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getRouterWrapper({ location: '/my-project' }),
      ])
    );

    await userEvent.click(screen.getByRole('button', { name: /menu/i }));
    await userEvent.click(screen.getByRole('link', { name: /settings/i }));

    expect(screen.getByText(/Current pathname: \/settings/)).toBeInTheDocument();
  });
});
