import { render, screen } from '@testing-library/react';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { MessageContainer } from '../message-container';

describe('MessageContainer', () => {
  it('renders the message text', () => {
    render(
      <MessageContainer message="Deployment succeeded." />,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.getByText('Deployment succeeded.')).toBeInTheDocument();
  });
});
