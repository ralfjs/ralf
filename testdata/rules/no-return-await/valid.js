async function foo() {
  return bar();
}
async function baz() {
  const result = await fetch("/api");
  return result;
}
async function qux() {
  return Promise.resolve(1);
}
