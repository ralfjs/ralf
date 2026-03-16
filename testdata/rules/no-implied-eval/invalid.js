setTimeout("alert('hi')", 100); // expect-error: no-implied-eval
setInterval("x++", 1000); // expect-error: no-implied-eval
execScript("code"); // expect-error: no-implied-eval
