async function foo() {
  return await bar(); // expect-error: no-return-await
}
async function baz() {
  return await fetch("/api"); // expect-error: no-return-await
}
async function qux() {
  return await Promise.resolve(1); // expect-error: no-return-await
}
