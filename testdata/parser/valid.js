import { readFile } from "fs";

const MAX_SIZE = 1024;

function greet(name) {
  return `Hello, ${name}!`;
}

class Logger {
  constructor(prefix) {
    this.prefix = prefix;
  }

  log(message) {
    console.log(`${this.prefix}: ${message}`);
  }
}

export { greet, Logger };
