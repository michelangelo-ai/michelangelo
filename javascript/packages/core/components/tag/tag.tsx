import { forwardRef } from 'react';
import { mergeOverrides, useStyletron } from 'baseui';
import { KIND as BASE_KIND, Tag as BaseTag } from 'baseui/tag';

import { TAG_BEHAVIOR, TAG_COLOR, TAG_HIERARCHY, TAG_SIZE } from './constants';
import { getTagOverrides } from './styled-components';

import type { TagKind as BaseTagKind, TagSize as BaseTagSize } from 'baseui/tag';
import type { Props } from './types';

export const Tag = forwardRef<HTMLElement, Props>(
  (
    {
      children,
      overrides,
      size = TAG_SIZE.small,
      color = TAG_COLOR.gray,
      behavior = TAG_BEHAVIOR.display,
      hierarchy = TAG_HIERARCHY.secondary,
      ...rest
    },
    ref
  ) => {
    const [_, theme] = useStyletron();

    const baseSize: BaseTagSize | undefined = size === TAG_SIZE.xSmall ? TAG_SIZE.small : size;
    const baseKind: BaseTagKind | undefined =
      color === TAG_COLOR.gray || color === TAG_COLOR.purple || color === TAG_COLOR.magenta
        ? BASE_KIND.black
        : color;

    return (
      <BaseTag
        {...rest}
        ref={ref}
        kind={baseKind}
        size={baseSize}
        hierarchy={hierarchy}
        overrides={mergeOverrides(
          overrides,
          getTagOverrides(theme, {
            size,
            color,
            behavior,
            hierarchy,
          })
        )}
      >
        {children}
      </BaseTag>
    );
  }
);

Tag.displayName = 'Tag';
