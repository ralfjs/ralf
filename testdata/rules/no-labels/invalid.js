outer: for (let i = 0; i < 10; i++) { // expect-error: no-labels
  break outer;
}
loop: while (true) { // expect-error: no-labels
  break loop;
}
block: do { // expect-error: no-labels
  break block;
} while (false);
