var a = "\8"; // expect-error: no-nonoctal-decimal-escape
var b = "\9"; // expect-error: no-nonoctal-decimal-escape
var c = "test\8value"; // expect-error: no-nonoctal-decimal-escape
