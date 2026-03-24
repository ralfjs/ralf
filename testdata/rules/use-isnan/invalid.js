if (x === NaN) {} // expect-error: use-isnan
if (NaN === x) {} // expect-error: use-isnan
if (x == NaN) {} // expect-error: use-isnan
