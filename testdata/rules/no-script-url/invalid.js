const a = "javascript:alert(1)"; // expect-error: no-script-url
const b = 'javascript:void(0)'; // expect-error: no-script-url
location.href = "javascript:doStuff()"; // expect-error: no-script-url
