import test from "node:test";
import assert from "node:assert/strict";
import { add, divide } from "../src/calculator.js";

test("add returns a sum", () => {
  assert.equal(add(2, 3), 5);
});

test("divide returns a quotient", () => {
  assert.equal(divide(8, 2), 4);
});

test("divide rejects division by zero", () => {
  assert.throws(() => divide(8, 0), /zero/i);
});
