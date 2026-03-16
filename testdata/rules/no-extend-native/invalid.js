Object.prototype.foo = function() {}; // expect-error: no-extend-native
Array.prototype.last = function() {}; // expect-error: no-extend-native
Object.defineProperty(Array.prototype, "last", {}); // expect-error: no-extend-native
