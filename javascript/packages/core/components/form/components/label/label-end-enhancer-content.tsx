import { useStyletron } from 'baseui';

import type { FormControlProps } from '#core/components/form/components/types';

/**
 * Renders content intended for FormControl's label end enhancer slot. Includes counter
 * display and/or arbitrary enhancer content, side-by-side.
 *
 * **Relies on parent FormControl's LabelEndEnhancer override for flex layout**
 */
export const LabelEndEnhancerContent = ({
  counter,
  labelEndEnhancer,
}: Pick<FormControlProps, 'counter' | 'labelEndEnhancer'>) => {
  const [css, theme] = useStyletron();

  return (
    <>
      {counter && (
        <span
          className={css({
            ...theme.typography.font100,
            color: theme.colors.contentPrimary,
          })}
        >
          {counter.length}/{counter.maxLength}
        </span>
      )}
      {labelEndEnhancer}
    </>
  );
};
