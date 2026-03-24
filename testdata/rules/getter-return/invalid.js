class A {
  get name() {} // expect-error: getter-return
}

class B {
  get value() { // expect-error: getter-return
    console.log("no return");
  }
}
