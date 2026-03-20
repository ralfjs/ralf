if (true) {
  function foo() {} // expect-error: no-inner-declarations
}

while (x) {
  function bar() {} // expect-error: no-inner-declarations
}
