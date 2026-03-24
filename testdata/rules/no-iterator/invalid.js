obj.__iterator__ = function() {}; // expect-error: no-iterator
const iter = foo.__iterator__; // expect-error: no-iterator
bar.__iterator__; // expect-error: no-iterator
obj["__iterator__"]; // expect-error: no-iterator
