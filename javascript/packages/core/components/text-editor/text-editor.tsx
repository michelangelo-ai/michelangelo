import { json } from '@codemirror/lang-json';
import { EditorView } from '@codemirror/view';
import CodeMirror from '@uiw/react-codemirror';
import { useStyletron } from 'baseui';

import type { TextEditorProps } from './types';

export function TextEditor(props: TextEditorProps) {
  const [css] = useStyletron();
  const { value, language, readOnly = false, height = '300px', onChange } = props;

  const extensions = language === 'json' ? [json()] : [];

  const themeExtension = EditorView.theme({
    '&': {
      fontSize: '14px',
      fontFamily: 'monospace',
    },
    '.cm-content': {
      padding: '12px',
    },
    '.cm-editor': {
      backgroundColor: readOnly ? '#F8F8F8' : '#FFFFFF',
    },
    '.cm-editor.cm-focused': {
      outline: 'none',
    },
  });

  return (
    <div
      className={css({
        border: '1px solid #E0E0E0',
        borderRadius: '4px',
        overflow: 'hidden',
        height,
      })}
    >
      <CodeMirror
        value={value}
        height={height}
        extensions={[...extensions, themeExtension]}
        onChange={onChange}
        editable={!readOnly}
        basicSetup={{
          lineNumbers: true,
          foldGutter: false,
          dropCursor: false,
          allowMultipleSelections: false,
          indentOnInput: false,
          bracketMatching: true,
          closeBrackets: false,
          autocompletion: false,
          highlightSelectionMatches: false,
          searchKeymap: false,
        }}
      />
    </div>
  );
}
