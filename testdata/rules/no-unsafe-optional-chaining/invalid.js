obj?.foo + 1; // expect-error: no-unsafe-optional-chaining
new obj?.Foo(); // expect-error: no-unsafe-optional-chaining
[...obj?.items]; // expect-error: no-unsafe-optional-chaining
(obj?.foo) + 1; // expect-error: no-unsafe-optional-chaining
