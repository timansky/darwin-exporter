const jsoncParser = require("jsonc-eslint-parser");
const jsoncPlugin = require("eslint-plugin-jsonc");

module.exports = [
  {
    files: ["docs/grafana/dashboard.json"],
    languageOptions: {
      parser: jsoncParser,
    },
    plugins: {
      jsonc: jsoncPlugin,
    },
    rules: {
      ...jsoncPlugin.configs["recommended-with-json"].rules,
      "jsonc/sort-keys": "off",
    },
  },
];
