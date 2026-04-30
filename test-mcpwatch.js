const { spawn } = require('child_process');
const path = require('path');

const proxy = spawn('node', ['index.js', 'run', '--command', 'node test-server.js'], {
  cwd: __dirname
});

proxy.stdout.on('data', (data) => {
  console.log(`Proxy Out: ${data}`);
});

proxy.stderr.on('data', (data) => {
  console.log(`Proxy Err: ${data}`);
});

setTimeout(() => {
  console.log('Sending ping...');
  proxy.stdin.write('{"jsonrpc": "2.0", "id": 1, "method": "ping"}\n');
}, 1000);

setTimeout(() => {
  console.log('Killing proxy...');
  proxy.kill();
}, 5000);
