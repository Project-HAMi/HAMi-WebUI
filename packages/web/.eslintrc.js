module.exports = {
  root: true,
  env: {
    node: true,
    browser: true,
    es2021: true,
    'vue/setup-compiler-macros': true,
  },
  extends: [
    'plugin:vue/vue3-essential',
    'eslint:recommended',
    'plugin:prettier/recommended',
  ],
  parserOptions: {
    parser: '@babel/eslint-parser',
    requireConfigFile: false,
    ecmaVersion: 2020,
    sourceType: 'module',
    babelOptions: {
      presets: ['@vue/cli-plugin-babel/preset'],
      plugins: ['@vue/babel-plugin-jsx'],
    },
  },
  rules: {
    'no-console': process.env.NODE_ENV === 'production' ? 'warn' : 'off',
    'no-debugger': process.env.NODE_ENV === 'production' ? 'warn' : 'off',
    'space-before-function-paren': 'off',
    'vue/multi-word-component-names': 'off',
    'prettier/prettier': 'off',
    'no-unused-vars': 'off',
    'no-empty': 'off',
    'no-useless-escape': 'off',
  },
  globals: {
    globals: 'readonly',
  },
};
