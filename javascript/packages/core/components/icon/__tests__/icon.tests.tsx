import { render, screen } from '@testing-library/react';
import { Alert } from 'baseui/icon';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { Icon } from '../icon';

describe('Icon', () => {
  it('should render icon registered in the icon provider', () => {
    render(
      <Icon name="check" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    expect(screen.getByText('Check')).toBeInTheDocument();
  });

  it('should render icon passed as a prop', () => {
    render(
      <Icon name="alert" icon={Alert} />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    expect(screen.getByText('Alert')).toBeInTheDocument();
  });

  it('should render empty if provided icon is not registered in the icon provider', () => {
    render(
      <Icon name="alert" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    expect(screen.queryByText('Alert')).not.toBeInTheDocument();
  });
});
