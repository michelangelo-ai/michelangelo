/** @type {import('eslint').Rule.RuleModule} */
const rule = {
  meta: {
    type: 'suggestion',
    docs: {
      description:
        "Require a '// cast:' comment explaining every type assertion",
      recommended: true,
      url: 'https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/eslint-local-rules/require-cast-comment.md',
    },
    messages: {
      missingCastComment:
        "Type assertion 'as {{ type }}' requires a '// cast:' comment. Add '// cast: <reason>' on the same line or the line above.",
    },
    schema: [],
  },

  create(context) {
    const src = context.getSourceCode?.() ?? context;

    function isSafeAssertion(node) {
      const t = node.typeAnnotation;
      // as unknown — TSUnknownKeyword (keyword type, not TSTypeReference)
      if (t.type === 'TSUnknownKeyword') return true;
      // as const — TSTypeReference with typeName 'const'
      if (
        t.type === 'TSTypeReference' &&
        t.typeName?.type === 'Identifier' &&
        t.typeName.name === 'const'
      )
        return true;
      return false;
    }

    function hasCastComment(node) {
      const lines = src.getText().split('\n');
      const line = node.loc.start.line; // 1-indexed

      // Same line
      if (lines[line - 1]?.includes('// cast:')) return true;

      // Line immediately above — blank lines break the connection
      if (line >= 2) {
        return lines[line - 2].trim().startsWith('// cast:');
      }

      return false;
    }

    return {
      TSAsExpression(node) {
        if (isSafeAssertion(node)) return;
        if (hasCastComment(node)) return;

        const typeText = src.getText(node.typeAnnotation);
        context.report({
          node,
          messageId: 'missingCastComment',
          data: { type: typeText },
        });
      },
    };
  },
};

export default rule;
