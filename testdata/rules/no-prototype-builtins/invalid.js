var result = foo.hasOwnProperty("bar"); // expect-error: no-prototype-builtins
obj.hasOwnProperty("key"); // expect-error: no-prototype-builtins
