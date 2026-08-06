import { render, screen } from '@testing-library/react';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { MessageContainer } from '../message-container';
import { MessageLevel } from '../types';

describe('MessageContainer', () => {
  it('renders the message text', () => {
    render(
      <MessageContainer message="Deployment succeeded." />,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.getByText('Deployment succeeded.')).toBeInTheDocument();
  });

  it('renders with info level by default', () => {
    const { container } = render(
      <MessageContainer message="Info message" />,
      buildWrapper([getBaseProviderWrapper()])
    );

    const root = container.firstChild as HTMLElement;
    expect(root).toBeInTheDocument();
  });

  it('renders with error level', () => {
    render(
      <MessageContainer message="Rollout failed." level={MessageLevel.ERROR} />,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.getByText('Rollout failed.')).toBeInTheDocument();
  });

  it('renders with warning level', () => {
    render(
      <MessageContainer message="Approaching limit." level={MessageLevel.WARNING} />,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.getByText('Approaching limit.')).toBeInTheDocument();
  });

  it('renders markdown content', () => {
    render(
      <MessageContainer message="**bold** and *italic*" />,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.getByText('bold')).toBeInTheDocument();
    expect(screen.getByText('italic')).toBeInTheDocument();
  });
});
