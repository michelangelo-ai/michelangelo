import { useEffect, useRef, useState } from 'react';

export function useScrollRatio<E extends HTMLElement = HTMLElement>(
  visibleColumns: unknown[]
): {
  scrollRatio: number;
  tableRef: React.RefObject<E | null>;
  handleScrollRatioUpdate: () => void;
} {
  const [scrollRatio, setScrollRatio] = useState(-1);
  const tableRef = useRef<E>(null);

  const handleScrollRatioUpdate = () => {
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

    handleScrollRatioUpdate();

    const resizeObserver = new ResizeObserver(handleScrollRatioUpdate);
    resizeObserver.observe(element);

    return () => resizeObserver.disconnect();
  }, [visibleColumns]);

  return { scrollRatio, tableRef, handleScrollRatioUpdate };
}
