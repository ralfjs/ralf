if (x === -0) {} // expect-error: no-compare-neg-zero
if (-0 === x) {} // expect-error: no-compare-neg-zero
if (x == -0) {} // expect-error: no-compare-neg-zero
