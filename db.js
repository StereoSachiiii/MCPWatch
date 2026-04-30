const Database = require('better-sqlite3');
const path = require('path');

const db = new Database(path.join(__dirname, 'mcpwatch.db'));

// Initialize schema
db.exec(`
  CREATE TABLE IF NOT EXISTS interactions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    direction TEXT, -- 'IN' (Agent -> Tool) or 'OUT' (Tool -> Agent)
    raw_message TEXT,
    method TEXT,
    params TEXT,
    result TEXT,
    error TEXT,
    jsonrpc_id TEXT
  );
`);

function logInteraction(direction, raw) {
  try {
    const data = JSON.parse(raw);
    const stmt = db.prepare(`
      INSERT INTO interactions (direction, raw_message, method, params, result, error, jsonrpc_id)
      VALUES (?, ?, ?, ?, ?, ?, ?)
    `);
    
    stmt.run(
      direction,
      raw,
      data.method || null,
      data.params ? JSON.stringify(data.params) : null,
      data.result ? JSON.stringify(data.result) : null,
      data.error ? JSON.stringify(data.error) : null,
      data.id ? String(data.id) : null
    );
  } catch (e) {
    const stmt = db.prepare(`
      INSERT INTO interactions (direction, raw_message)
      VALUES (?, ?)
    `);
    stmt.run(direction, raw);
  }
}

function getInteractions(limit = 20) {
  return db.prepare('SELECT * FROM interactions ORDER BY id DESC LIMIT ?').all(limit);
}

function clearLogs() {
  db.prepare('DELETE FROM interactions').run();
}

module.exports = {
  logInteraction,
  getInteractions,
  clearLogs
};
