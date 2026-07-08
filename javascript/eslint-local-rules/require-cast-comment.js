/** @type {import('eslint').Rule.RuleModule} */
const rule = {
  meta: {
    type: 'suggestion',
    docs: {
      description: "Require a '// cast:' comment explaining every type assertion",
      recommended: true,
      url: 'https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/eslint-local-rules/require-cast-comment.md',
    },
    messages: {
      missingCastComment:
        "Type assertion 'as {{ type }}' requires a '// cast:' comment. Add '// cast: <reason>' anywhere in the unbroken comment block on the line(s) above.",
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

    function commentHasCastMarker(rawLine) {
      const trimmed = rawLine.trim();
      if (!trimmed.startsWith('//')) return null; // not a comment line — caller stops walking
      return trimmed.replace(/^\/\/\s*/, '').startsWith('cast:');
    }

    function hasCastComment(node) {
      const lines = src.getText().split('\n');
      const line = node.loc.start.line; // 1-indexed

      // Contiguous block of `//` comment lines immediately above — a blank
      // line or a non-comment line breaks the chain.
      for (let i = line - 2; i >= 0; i--) {
        const result = commentHasCastMarker(lines[i]);
        if (result === null) break;
        if (result) return true;
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
