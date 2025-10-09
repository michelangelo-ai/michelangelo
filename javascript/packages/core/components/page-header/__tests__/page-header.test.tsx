import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Alert } from 'baseui/icon';
import { vi } from 'vitest';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { PageHeader } from '../page-header';

describe('PageHeader', () => {
  const originalWindowOpen = window.open;

  beforeEach(() => {
    window.open = vi.fn();
  });

  afterEach(() => {
    window.open = originalWindowOpen;
  });

  test('renders label text', () => {
    render(<PageHeader label="My Page Title" />, buildWrapper([getBaseProviderWrapper()]));

    expect(screen.getByRole('heading', { name: 'My Page Title' })).toBeInTheDocument();
  });

  test('renders icon when provided', () => {
    render(
      <PageHeader label="Train & Evaluate" icon="chartLine" />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper({ icons: { chartLine: Alert } }),
      ])
    );

    expect(screen.getByRole('img', { name: 'Alert' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Train & Evaluate' })).toBeInTheDocument();
  });

  test('renders description when provided', () => {
    render(
      <PageHeader label="Title" description="This is a description" />,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.getByText('This is a description')).toBeInTheDocument();
  });

  test('renders documentation button when docUrl is provided with description', () => {
    render(
      <PageHeader label="Title" description="Description text" docUrl="https://docs.example.com" />,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.getByRole('button', { name: /learn more/i })).toBeInTheDocument();
  });

  test('does not render documentation button when docUrl provided without description', () => {
    render(
      <PageHeader label="Title" docUrl="https://docs.example.com" />,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  test('opens documentation URL in new window when button clicked', async () => {
    const user = userEvent.setup();

    render(
      <PageHeader
        label="Title"
        description="Description"
        docUrl="https://docs.example.com/guide"
      />,
      buildWrapper([getBaseProviderWrapper()])
    );

    await user.click(screen.getByRole('button', { name: /learn more/i }));

    expect(window.open).toHaveBeenCalledWith('https://docs.example.com/guide', '_blank');
  });
});
