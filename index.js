#!/usr/bin/env node
const { spawn } = require('child_process');
const yargs = require('yargs/yargs');
const { hideBin } = require('yargs/helpers');
const { logInteraction, getInteractions, clearLogs } = require('./db');
const chalk = require('chalk');

const argv = yargs(hideBin(process.argv))
  .command('run', 'Wrap an MCP server and log interactions', (y) => {
    return y.option('command', {
      alias: 'c',
      type: 'string',
      description: 'The command to run'
    });
  })
  .command('list', 'List recent interactions', (y) => {
    return y.option('limit', {
      alias: 'l',
      type: 'number',
      default: 10
    });
  })
  .command('clear', 'Clear all logged interactions')
  .demandCommand(1)
  .help()
  .argv;

if (argv._[0] === 'run') {
  const command = argv.command || argv._[1];
  const args = argv.command ? (argv.args || []) : argv._.slice(2);
  
  if (!command) {
    console.error(chalk.red('Error: No command specified. Use --command or "mcpwatch run -- command args"'));
    process.exit(1);
  }
  
  startProxy(command, args);
} else if (argv._[0] === 'list') {
  showList(argv.limit);
} else if (argv._[0] === 'clear') {
  clearLogs();
  console.log(chalk.green('Logs cleared.'));
}

function startProxy(command, args) {
  console.error(chalk.blue(`[MCPWatch] Starting proxy for: ${command} ${args.join(' ')}`));

  const child = spawn(command, args, {
    stdio: ['pipe', 'pipe', 'inherit'],
    shell: true
  });

  let inBuffer = '';
  let outBuffer = '';

  function processStream(chunk, direction, buffer, targetStream) {
    const str = chunk.toString();
    if (targetStream) targetStream.write(chunk);
    
    buffer += str;
    let lines = buffer.split('\n');
    buffer = lines.pop();
    
    for (const line of lines) {
      if (line.trim()) {
        logInteraction(direction, line.trim());
      }
    }
    return buffer;
  }

  process.stdin.on('data', (chunk) => {
    inBuffer = processStream(chunk, 'IN', inBuffer, child.stdin);
  });

  child.stdout.on('data', (chunk) => {
    outBuffer = processStream(chunk, 'OUT', outBuffer, process.stdout);
  });

  child.on('close', (code) => {
    console.error(chalk.yellow(`[MCPWatch] Child process exited with code ${code}`));
    process.exit(code);
  });

  process.on('SIGINT', () => {
    child.kill('SIGINT');
  });
}

function showList(limit) {
  const interactions = getInteractions(limit);
  console.log(chalk.bold.underline(`\nRecent Interactions (Last ${limit}):\n`));
  
  interactions.reverse().forEach(i => {
    const time = chalk.gray(new Date(i.timestamp).toLocaleTimeString());
    const isOut = i.direction === 'OUT';
    const dirIcon = isOut ? chalk.cyan('⇠') : chalk.green('➔');
    const label = isOut ? chalk.cyan('TOOL -> AGENT') : chalk.green('AGENT -> TOOL');
    
    let info = i.method ? chalk.yellow(i.method) : '';
    const id = i.jsonrpc_id ? chalk.magenta(`[id:${i.jsonrpc_id}]`) : '';
    
    console.log(`${time} ${dirIcon} ${label} ${info} ${id}`);
    
    if (i.params) {
        try {
            const p = JSON.parse(i.params);
            const summary = JSON.stringify(p).length > 120 ? JSON.stringify(p).substring(0, 120) + '...' : JSON.stringify(p);
            console.log(chalk.gray(`    Params: ${summary}`));
        } catch(e) {}
    }
    if (i.result) {
        try {
            const r = JSON.parse(i.result);
            const summary = JSON.stringify(r).length > 120 ? JSON.stringify(r).substring(0, 120) + '...' : JSON.stringify(r);
            console.log(chalk.gray(`    Result: ${summary}`));
        } catch(e) {}
    }
    if (i.error) {
        console.log(chalk.red(`    Error: ${i.error}`));
    }
  });
  console.log('\n');
}
