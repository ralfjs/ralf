switch (x) {
  case 1:
    let a = 1; // expect-error: no-case-declarations
    break;
  case 2:
    const b = 2; // expect-error: no-case-declarations
    break;
}
