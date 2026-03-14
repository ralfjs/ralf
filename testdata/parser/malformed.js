function broken() {
  const x = "unterminated string;
  if (true) {
    console.log("missing closing brace")
