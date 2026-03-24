var re = /\x00/; // expect-error: no-control-regex
var re2 = /\x1f/; // expect-error: no-control-regex
var re3 = /\u001f/; // expect-error: no-control-regex
