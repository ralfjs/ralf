switch (x) {
  case 1: // expect-error: no-fallthrough
    foo();
  case 2:
    break;
}
