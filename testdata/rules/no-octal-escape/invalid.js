const a = "\251"; // expect-error: no-octal-escape
const b = "\1"; // expect-error: no-octal-escape
const c = "\77"; // expect-error: no-octal-escape
const d = "\00"; // expect-error: no-octal-escape
