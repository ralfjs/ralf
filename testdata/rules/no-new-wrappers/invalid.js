const s = new String("hello"); // expect-error: no-new-wrappers
const n = new Number(42); // expect-error: no-new-wrappers
const b = new Boolean(true); // expect-error: no-new-wrappers
const x = new String; // expect-error: no-new-wrappers
