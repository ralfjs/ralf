const proto = obj.__proto__; // expect-error: no-proto
obj.__proto__ = null; // expect-error: no-proto
foo.bar.__proto__; // expect-error: no-proto
obj["__proto__"]; // expect-error: no-proto
