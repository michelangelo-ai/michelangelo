import { render, screen } from '@testing-library/react';

import { TruncatedText } from '../truncated-text';

describe('TruncatedText', () => {
  test('renders short text', () => {
    render(<TruncatedText>test</TruncatedText>);

    expect(screen.getByText('test')).toBeInTheDocument();
  });
});
