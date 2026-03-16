const proto = Object.getPrototypeOf(obj);
Object.setPrototypeOf(obj, null);
const c = { __proto__: proto };
const x = 1;
