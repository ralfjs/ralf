function f(a, b, a) {} // expect-error: no-dupe-args
function g(x, x) {} // expect-error: no-dupe-args
function h({a}, {a}) {} // expect-error: no-dupe-args
