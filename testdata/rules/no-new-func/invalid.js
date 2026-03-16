const f = new Function("a", "return a"); // expect-error: no-new-func
const g = Function("return 1"); // expect-error: no-new-func
const h = new Function("a", "b", "return a + b"); // expect-error: no-new-func
