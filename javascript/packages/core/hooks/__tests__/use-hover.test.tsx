import { useRef } from 'react';
import { render, renderHook, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { useHover } from '../use-hover';

function TestHoverComponent() {
  const ref = useRef<HTMLDivElement>(null);
  const isHovered = useHover(ref);

  return (
    <div>
      <div
        ref={ref}
        data-testid="hover-target"
        style={{ width: '100px', height: '100px', backgroundColor: 'blue' }}
      >
        Hover me
      </div>
      <div data-testid="hover-status">{isHovered ? 'hovered' : 'not-hovered'}</div>
    </div>
  );
}

describe('useHover', () => {
  it('should return false initially', () => {
    const ref = { current: null };
    const { result } = renderHook(() => useHover(ref));

    expect(result.current).toBe(false);
  });

  it('should respond to mouse events', async () => {
    const user = userEvent.setup();
    render(<TestHoverComponent />);

    const target = screen.getByTestId('hover-target');
    const status = screen.getByTestId('hover-status');

    expect(status.textContent).toBe('not-hovered');

    await user.hover(target);
    expect(status.textContent).toBe('hovered');

    await user.unhover(target);
    expect(status.textContent).toBe('not-hovered');
  });

  it('should clean up event listeners on unmount', () => {
    const addEventListenerSpy = vi.fn();
    const removeEventListenerSpy = vi.fn();

    const mockElement = {
      addEventListener: addEventListenerSpy,
      removeEventListener: removeEventListenerSpy,
    } as unknown as HTMLElement;

    const ref = { current: mockElement };
    const { unmount } = renderHook(() => useHover(ref));

    expect(addEventListenerSpy).toHaveBeenCalledTimes(2);
    expect(addEventListenerSpy).toHaveBeenCalledWith('mouseenter', expect.any(Function));
    expect(addEventListenerSpy).toHaveBeenCalledWith('mouseleave', expect.any(Function));

    unmount();

    expect(removeEventListenerSpy).toHaveBeenCalledTimes(2);
    expect(removeEventListenerSpy).toHaveBeenCalledWith('mouseenter', expect.any(Function));
    expect(removeEventListenerSpy).toHaveBeenCalledWith('mouseleave', expect.any(Function));
  });
});
