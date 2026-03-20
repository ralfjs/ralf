class A {
  foo() {}
  foo() {} // expect-error: no-dupe-class-members
}

class B {
  bar() {}
  get baz() {}
  bar() {} // expect-error: no-dupe-class-members
}
