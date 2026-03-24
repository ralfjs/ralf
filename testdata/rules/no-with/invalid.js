with (obj) { // expect-error: no-with
  x = 1;
}
with (Math) { // expect-error: no-with
  y = cos(0);
}
with (document) { // expect-error: no-with
  z = getElementById("a");
}
