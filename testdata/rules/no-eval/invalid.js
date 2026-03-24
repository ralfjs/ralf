eval("code"); // expect-error: no-eval
eval("1 + 2"); // expect-error: no-eval
const x = eval("3"); // expect-error: no-eval
(0, eval)("indirect"); // expect-error: no-eval
const y = 1;
