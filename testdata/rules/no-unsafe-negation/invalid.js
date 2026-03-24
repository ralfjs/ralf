if (!key in obj) {} // expect-error: no-unsafe-negation
if (!obj instanceof Foo) {} // expect-error: no-unsafe-negation
