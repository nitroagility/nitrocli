const { greet, sleep } = require("./index");
const assert = require("assert");

assert.strictEqual(greet("World"), "Hello, World!");
assert.ok(sleep(1) instanceof Promise);

console.log("all tests passed");
