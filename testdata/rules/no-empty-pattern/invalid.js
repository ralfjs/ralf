var {} = foo; // expect-error: no-empty-pattern
var [] = bar; // expect-error: no-empty-pattern
function f({}) {} // expect-error: no-empty-pattern
