import React, { useCallback, useRef, useState } from 'react';
import { Dialog as BaseDialog } from 'baseui/dialog';

import { mergeAllOverrides } from '#core/utils/style-utils';
import { LAYER_HEADER_ABOVE_CONTENTS, enableButtonDockShadow } from './styled-components';

import type { DialogProps } from 'baseui/dialog';

export const Dialog: React.FC<DialogProps> = (props) => {
  const { overrides = {}, ...rest } = props;

  const previousScrollRef = useRef<HTMLDivElement | null>(null);
  const [hasScrolledToBottom, setHasScrolledToBottom] = useState(true);

  const setScrollRef = useCallback((node: HTMLDivElement | null) => {
    const handleScroll = () => {
      if (previousScrollRef.current) {
        const { clientHeight, scrollHeight, scrollTop } = previousScrollRef.current;
        setHasScrolledToBottom(clientHeight + scrollTop === scrollHeight);
      }
    };

    if (node) {
      const { clientHeight, scrollHeight } = node;
      setHasScrolledToBottom(clientHeight >= scrollHeight);

      node.addEventListener('scroll', handleScroll);
    }

    previousScrollRef.current = node;
  }, []);

  return (
    <BaseDialog
      {...rest}
      overrides={mergeAllOverrides(
        overrides,
        LAYER_HEADER_ABOVE_CONTENTS,
        enableButtonDockShadow(hasScrolledToBottom),
        { ScrollContainer: { props: { ref: setScrollRef } } }
      )}
    />
  );
};
