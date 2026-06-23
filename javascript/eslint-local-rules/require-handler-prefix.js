/**
 * @fileoverview Requires locally-defined event handlers passed to on* props to use the handle* prefix.
 *
 * Passthrough props — values forwarded directly from component parameters — are exempt.
 * The handle* naming already applies at the call site where the handler was defined.
 *
 * Does not apply to test files (configure in eslint.config.js).
 */

/** @type {import('eslint').Rule.RuleModule} */
const rule = {
  meta: {
    type: 'suggestion',
    docs: {
      description:
        'Require handle* prefix for locally-defined functions passed to on* event props',
      recommended: true,
      url: 'https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/eslint-local-rules/require-handler-prefix.md',
    },
    messages: {
      requireHandlerPrefix:
        "'{{valueName}}' is passed as handler for '{{propName}}' but does not start with 'handle'. " +
        "Rename to 'handle...' to indicate it is an event handler.",
    },
    schema: [],
  },

  create(context) {
    function isPassThroughProp(identifierNode) {
      const name = identifierNode.name;
      let scope = context.sourceCode.getScope(identifierNode);
      while (scope) {
        for (const variable of scope.variables) {
          if (variable.name === name) {
            if (variable.defs.length === 0) return false;
            const def = variable.defs[0];
            // Direct parameter destructuring: ({ onClose }) => ...
            if (def.type === 'Parameter') return true;
            // Indirect: const { onClose } = props where props is a parameter.
            // Covers forwardRef's (props, ref) => { const { onClose } = props } pattern.
            if (def.type === 'Variable' && def.node.init?.type === 'Identifier') {
              const sourceName = def.node.init.name;
              let innerScope = scope;
              while (innerScope) {
                for (const v of innerScope.variables) {
                  if (
                    v.name === sourceName &&
                    v.defs.length > 0 &&
                    v.defs[0].type === 'Parameter'
                  ) {
                    return true;
                  }
                }
                innerScope = innerScope.upper;
              }
            }
            return false;
          }
        }
        scope = scope.upper;
      }
      return false;
    }

    return {
      JSXAttribute(node) {
        if (node.name.type !== 'JSXIdentifier') return;
        const propName = node.name.name;
        if (!propName.startsWith('on')) return;

        if (!node.value || node.value.type !== 'JSXExpressionContainer') return;
        if (node.value.expression.type !== 'Identifier') return;

        if (isPassThroughProp(node.value.expression)) return;

        const valueName = node.value.expression.name;
        if (!valueName.startsWith('handle')) {
          context.report({
            node,
            messageId: 'requireHandlerPrefix',
            data: { propName, valueName },
          });
        }
      },
    };
  },
};

export default rule;
