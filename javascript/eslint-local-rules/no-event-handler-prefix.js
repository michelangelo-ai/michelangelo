/**
 * @fileoverview Disallows event handler names that mirror the prop without adding context.
 *
 * onClick={onClick}, onChange={handleChange}, onClick={handleOnClick} all tell
 * the reader nothing about *what* is being handled. Descriptive names like
 * onChange={handleRowChange} or onChange={commitSelection} make the intent clear.
 */

const capitalize = (str) => str.charAt(0).toUpperCase() + str.slice(1);

/** @type {import('eslint').Rule.RuleModule} */
const rule = {
  meta: {
    type: 'suggestion',
    docs: {
      description: 'Disallow event handler names that mirror the prop name without adding context',
      recommended: true,
      url: 'https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/eslint-local-rules/no-event-handler-prefix.md',
    },
    messages: {
      noHandlerMirror:
        "'{{valueName}}' mirrors the prop '{{propName}}' without adding context. " +
        'Use a name that describes what is handled (e.g. handleRowChange instead of handleChange).',
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
            return variable.defs.length > 0 && variable.defs[0].type === 'Parameter';
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

        // Skip props that are forwarded directly from the component's own parameters
        if (isPassThroughProp(node.value.expression)) return;

        const valueName = node.value.expression.name;
        const eventName = propName.slice(2); // 'onChange' -> 'Change'

        const mirrors = [
          propName, // onClick={onClick}
          `handle${capitalize(eventName)}`, // onChange={handleChange}
          `handle${capitalize(propName)}`, // onClick={handleOnClick}
        ];

        if (mirrors.includes(valueName)) {
          context.report({
            node,
            messageId: 'noHandlerMirror',
            data: { propName, valueName },
          });
        }
      },
    };
  },
};

export default rule;
