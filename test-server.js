#!/usr/bin/env node
const readline = require('readline');

const rl = readline.createInterface({
  input: process.stdin,
  terminal: false
});

rl.on('line', (line) => {
  try {
    const request = JSON.parse(line);
    if (request.method === 'ping') {
      console.log(JSON.stringify({
        jsonrpc: '2.0',
        id: request.id,
        result: 'pong'
      }));
    } else {
      console.log(JSON.stringify({
        jsonrpc: '2.0',
        id: request.id,
        result: { message: `Echo: ${request.method}` }
      }));
    }
  } catch (e) {
    // Ignore invalid JSON
  }
});
