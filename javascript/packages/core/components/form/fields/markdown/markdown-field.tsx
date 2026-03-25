import { useStyletron } from 'baseui';
import { Textarea } from 'baseui/textarea';

import { FormControl } from '#core/components/form/components/form-control';
import { useField } from '#core/components/form/hooks/use-field';
import { Markdown } from '#core/components/markdown/markdown';

import type { MarkdownFieldProps } from './types';

/**
 * Markdown-aware text field with dual rendering modes.
 *
 * In edit mode, displays a plain textarea for raw markdown input.
 * In read-only mode, renders the stored value as formatted markdown.
 */
export const MarkdownField: React.FC<MarkdownFieldProps> = ({
  name,
  label,
  defaultValue,
  initialValue,
  required,
  validate,
  readOnly,
  disabled,
  placeholder,
  description,
  caption,
  labelEndEnhancer,
  format,
  parse,
  rows,
  maxLength,
}) => {
  const { input, meta } = useField<string>(name, {
    required,
    validate,
    defaultValue,
    initialValue,
    label,
    format,
    parse,
  });
  const [css, theme] = useStyletron();
  const currentLength = input.value?.length ?? 0;

  return (
    <FormControl
      label={label}
      required={required}
      description={description}
      labelEndEnhancer={labelEndEnhancer}
      caption={caption}
      error={meta.touched && meta.error ? meta.error : undefined}
      counter={maxLength ? { length: currentLength, maxLength } : undefined}
    >
      {readOnly ? (
        <div
          className={css({
            ...theme.typography.font300,
            // To prevent the div from collapsing when the content is empty
            minHeight: String(theme.typography.font300.lineHeight),
            border: `${theme.sizing.scale0} solid ${theme.colors.borderOpaque}`,
            borderRadius: theme.borders.radius300,
            paddingTop: theme.sizing.scale400,
            paddingBottom: theme.sizing.scale400,
            paddingLeft: theme.sizing.scale550,
            paddingRight: theme.sizing.scale550,
          })}
        >
          <Markdown>{input.value}</Markdown>
        </div>
      ) : (
        <Textarea
          id={input.name}
          name={input.name}
          value={input.value}
          onChange={(e) => input.onChange(e.currentTarget.value)}
          onBlur={input.onBlur}
          onFocus={input.onFocus}
          placeholder={placeholder}
          disabled={disabled}
          rows={rows}
          maxLength={maxLength}
          overrides={{
            Input: {
              style: {
                resize: 'vertical',
              },
            },
          }}
        />
      )}
    </FormControl>
  );
};
