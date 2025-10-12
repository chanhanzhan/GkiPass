module.exports = {
  extends: "next/core-web-vitals",
  rules: {
    "@typescript-eslint/no-explicit-any": "on",
    "react/no-unescaped-entities": "on",
    "react-hooks/exhaustive-deps": "on",
    "@typescript-eslint/no-unused-vars": "on",
    "@typescript-eslint/no-empty-interface": "on",
    "@typescript-eslint/no-empty-object-type": "on"
  },
  ignorePatterns: ["node_modules/", ".next/"]
}
