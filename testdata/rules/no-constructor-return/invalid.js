class A {
  constructor() {
    return this; // expect-error: no-constructor-return
  }
}

class B {
  constructor() {
    return 42; // expect-error: no-constructor-return
  }
}
