import { render, screen } from '@testing-library/react';

import { TextCell } from '../text-cell';

describe('TextCell', () => {
  it('should render text content', () => {
    render(<TextCell column={{ id: 'test' }} record={{}} value="Test content" />);

    expect(screen.getByText('Test content')).toBeInTheDocument();
  });

  it('should render em dash when value is undefined', () => {
    render(<TextCell column={{ id: 'test' }} record={{}} value={undefined} />);

    expect(screen.getByText('\u2014')).toBeInTheDocument();
  });

  it('should render em dash when value is empty string', () => {
    render(<TextCell column={{ id: 'test' }} record={{}} value="" />);

    expect(screen.getByText('\u2014')).toBeInTheDocument();
  });
});
