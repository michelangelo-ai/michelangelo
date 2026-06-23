import { RuleTester } from 'eslint';

import rule from '../no-handler-mirror.js';

RuleTester.describe = describe;
RuleTester.it = it;

const tester = new RuleTester({
  languageOptions: {
    ecmaVersion: 2020,
    sourceType: 'module',
    parserOptions: {
      ecmaFeatures: { jsx: true },
    },
  },
});

tester.run('no-handler-mirror', rule, {
  valid: [
    {
      name: 'descriptive name — not a mirror',
      code: `const C = () => <Input onChange={commitSelection} />;`,
    },
    {
      name: 'member expression pass-through',
      code: `const C = (props) => <Button onClick={props.onClick} />;`,
    },
    {
      name: 'non-event prop is ignored',
      code: `const C = () => <Button label={label} />;`,
    },
    {
      name: 'prop forwarded from parameter destructuring — onClick pass-through',
      code: `const C = ({ onClick }) => <Button onClick={onClick} />;`,
    },
    {
      name: 'prop forwarded from parameter destructuring — onChange pass-through',
      code: `const C = ({ onChange }) => <Input onChange={onChange} />;`,
    },
    {
      name: 'aliased destructuring is still a parameter — exempt',
      code: `const C = ({ onClose: handleClose }) => <Modal onClose={handleClose} />;`,
    },
    {
      name: 'persistInput names the effect, not the trigger',
      code: `const C = () => <Input onChange={persistInput} />;`,
    },
    {
      name: 'deleteItem names the trigger, not the effect',
      code: `const C = () => <Button onClick={deleteItem} />;`,
    },
    {
      name: 'handle + descriptive suffix — handleCommitSelection is valid for onChange',
      code: `const C = () => <Select onChange={handleCommitSelection} />;`,
    },
  ],

  invalid: [
    {
      name: 'onClick={onClick} — mirrors prop name exactly',
      code: `const handleClick = () => {}; const C = () => <Button onClick={handleClick} />;`,
      errors: [{ messageId: 'noHandlerMirror' }],
    },
    {
      name: 'onChange={handleChange} — handle + eventName',
      code: `const handleChange = () => {}; const C = () => <Input onChange={handleChange} />;`,
      errors: [{ messageId: 'noHandlerMirror' }],
    },
    {
      name: 'onSubmit={handleSubmit} — handle + eventName',
      code: `const handleSubmit = () => {}; const C = () => <Form onSubmit={handleSubmit} />;`,
      errors: [{ messageId: 'noHandlerMirror' }],
    },
    {
      name: 'onClick={handleOnClick} — handle + full prop name',
      code: `const handleOnClick = () => {}; const C = () => <Button onClick={handleOnClick} />;`,
      errors: [{ messageId: 'noHandlerMirror' }],
    },
    {
      name: 'onSubmit={onSubmit} — locally-declared var that mirrors prop',
      code: `const onSubmit = () => {}; const C = () => <Form onSubmit={onSubmit} />;`,
      errors: [{ messageId: 'noHandlerMirror' }],
    },
    {
      name: 'known limitation — destructured member expression pass-through',
      code: `const C = (props) => { const { onClick } = props; return <Button onClick={onClick} />; }`,
      errors: [{ messageId: 'noHandlerMirror' }],
    },
  ],
});
