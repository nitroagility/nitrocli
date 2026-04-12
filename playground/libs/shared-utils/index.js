function greet(name) {
  return `Hello, ${name}!`;
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

module.exports = { greet, sleep };
