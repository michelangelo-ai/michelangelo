import { useEffect, useRef, useState } from 'react';

export function useScrollRatio<T>(visibleColumns: T[]): {
  scrollRatio: number;
  tableRef: React.RefObject<HTMLElement | null>;
  updateScrollRatio: () => void;
} {
  const [scrollRatio, setScrollRatio] = useState(-1);
  const tableRef = useRef<HTMLElement>(null);

  const updateScrollRatio = () => {
    const element = tableRef.current;
    if (!element) return;

    const { scrollWidth, clientWidth, scrollLeft } = element;
    const containerWidth = scrollWidth - clientWidth;

    if (containerWidth === 0) {
      setScrollRatio(-1);
    } else {
      setScrollRatio(Math.round(scrollLeft) / containerWidth);
    }
  };

  useEffect(() => {
    const element = tableRef.current;
    if (!element) return;

    updateScrollRatio();

    const resizeObserver = new ResizeObserver(updateScrollRatio);
    resizeObserver.observe(element);

    return () => resizeObserver.disconnect();
  }, [visibleColumns]);

  return { scrollRatio, tableRef, updateScrollRatio };
}
