if (true) {} // expect-error: no-constant-condition
while (1) {} // expect-error: no-constant-condition
if ("hello") {} // expect-error: no-constant-condition
if (-1) {} // expect-error: no-constant-condition
