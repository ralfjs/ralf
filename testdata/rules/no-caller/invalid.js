const caller = arguments.caller; // expect-error: no-caller
const callee = arguments.callee; // expect-error: no-caller
function f() { return arguments.callee; } // expect-error: no-caller
