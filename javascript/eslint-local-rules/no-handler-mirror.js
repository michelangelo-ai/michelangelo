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
      url: 'https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/eslint-local-rules/no-handler-mirror.md',
    },
    messages: {
      noHandlerMirror:
        "'{{valueName}}' mirrors the prop '{{propName}}' without adding context. " +
        'Use a name that describes what is handled (e.g. handleRowChange instead of handleChange).',
    },
    schema: [],
  },

  create(context) {
    function isParamInScope(name, scope) {
      let s = scope;
      while (s) {
        for (const v of s.variables) {
          if (v.name === name && v.defs.length > 0 && v.defs[0].type === 'Parameter') return true;
        }
        s = s.upper;
      }
      return false;
    }

    function extractIdentifiers(node) {
      if (!node) return [];
      if (node.type === 'Identifier') return [node.name];
      if (node.type === 'LogicalExpression') return [...extractIdentifiers(node.left), ...extractIdentifiers(node.right)];
      if (node.type === 'MemberExpression') return extractIdentifiers(node.object);
      if (node.type === 'ChainExpression') return extractIdentifiers(node.expression);
      return [];
    }

    function isPassThroughProp(identifierNode) {
      const name = identifierNode.name;
      let scope = context.sourceCode.getScope(identifierNode);
      while (scope) {
        for (const variable of scope.variables) {
          if (variable.name === name) {
            if (variable.defs.length === 0) return false;
            const def = variable.defs[0];
            if (def.type === 'Parameter') return true;
            if (def.type === 'Variable' && def.node.init) {
              const identifiers = extractIdentifiers(def.node.init);
              if (identifiers.some((id) => isParamInScope(id, scope))) return true;
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
