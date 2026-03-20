class A {
  get name() {
    return this._name;
  }
}

class B {
  get value() {
    if (this._value) {
      return this._value;
    }
    return null;
  }
}

class C {
  set name(v) {
    this._name = v;
  }
}

class D {
  method() {}
}

class E {
  get() {
    return 1;
  }
}

class F {
  static get() {
    return 2;
  }
}
