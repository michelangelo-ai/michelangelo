import { useEffect, useRef, useState } from 'react';

import { StickyFooterContainer, StickyFooterSlot, StickyFooterSpacer } from './styled-components';

import type { StickyFooterProps } from './types';

export function StickyFooter({ leftContent, rightContent }: StickyFooterProps) {
  const footerRef = useRef<HTMLElement>(null);
  const [footerHeight, setFooterHeight] = useState(0);

  // Dynamically track footer height so the spacer always matches, preventing
  // content from being hidden behind the fixed footer when scrolled to bottom.
  useEffect(() => {
    const footer = footerRef.current;
    if (!footer) return;

    const observer = new ResizeObserver(() => {
      setFooterHeight(footer.offsetHeight);
    });

    observer.observe(footer);
    return () => observer.disconnect();
  }, []);

  return (
    <>
      <StickyFooterSpacer $height={footerHeight} />
      <StickyFooterContainer ref={footerRef}>
        <StickyFooterSlot>{leftContent}</StickyFooterSlot>
        <StickyFooterSlot>{rightContent}</StickyFooterSlot>
      </StickyFooterContainer>
    </>
  );
}
