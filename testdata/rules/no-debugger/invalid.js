debugger; // expect-error: no-debugger
function test() {
  debugger; // expect-error: no-debugger
  return 1;
}
debugger; // expect-error: no-debugger
