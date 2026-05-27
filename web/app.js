/* MCPWatch — Dashboard Application Logic */

(function () {
    'use strict';

    // ── State ──
    const state = {
        messages: [],
        connected: false,
        ws: null,
        filter: { search: '', direction: '', transport: '' },
        expandedId: null,
    };

    // ── DOM References ──
    const $ = (sel) => document.querySelector(sel);
    const timeline = $('#timeline');
    const searchInput = $('#search-input');
    const statusDot = $('#status-dot');
    const statusText = $('#status-text');
    const clientCount = $('#client-count');

    const statTotal = $('#stat-total');
    const statRequests = $('#stat-requests');
    const statLatency = $('#stat-latency');
    const statErrors = $('#stat-errors');
    const statVolume = $('#stat-volume');
    const statTokens = $('#stat-tokens');

    // ── WebSocket Connection ──
    function connect() {
        const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
        const ws = new WebSocket(`${proto}//${location.host}/ws`);

        ws.onopen = () => {
            state.connected = true;
            state.ws = ws;
            updateConnectionStatus();
        };

        ws.onmessage = (event) => {
            try {
                const msg = JSON.parse(event.data);
                state.messages.unshift(msg);
                if (state.messages.length > 500) state.messages.pop();
                renderNewMessage(msg);
                updateStats();
            } catch (e) {
                console.error('[MCPWatch] Parse error:', e);
            }
        };

        ws.onclose = () => {
            state.connected = false;
            state.ws = null;
            updateConnectionStatus();
            setTimeout(connect, 2000);
        };

        ws.onerror = () => ws.close();
    }

    // ── Initial Data Load ──
    async function loadHistory() {
        try {
            const res = await fetch('/api/interactions');
            const data = await res.json();
            if (data && data.length) {
                state.messages = data;
                renderTimeline();
            }
            updateStats();
        } catch (e) {
            console.error('[MCPWatch] Failed to load history:', e);
        }
    }

    // ── Rendering ──
    function renderTimeline() {
        const filtered = getFilteredMessages();
        if (filtered.length === 0) {
            timeline.innerHTML = `
                <div class="timeline-empty">
                    <div class="icon">◇</div>
                    <div>waiting for json-rpc traffic...</div>
                    <div style="margin-top:6px;font-size:0.78rem;color:var(--text-muted)">
                        start your mcp server with mcpwatch --wrap
                    </div>
                </div>`;
            return;
        }
        timeline.innerHTML = filtered.map((m, i) => buildCard(m, i)).join('');
    }

    function renderNewMessage(msg) {
        if (!matchesFilter(msg)) return;

        const empty = timeline.querySelector('.timeline-empty');
        if (empty) empty.remove();

        const div = document.createElement('div');
        div.innerHTML = buildCard(msg, 0);
        const card = div.firstElementChild;

        timeline.prepend(card);

        // Cap visible cards
        while (timeline.children.length > 200) {
            timeline.lastChild.remove();
        }
    }

    function buildCard(msg, index) {
        const time = formatTime(msg.timestamp);
        const dir = (msg.direction || '').toUpperCase();
        const dirClass = dir === 'IN' ? 'in' : 'out';
        const dirLabel = dir === 'IN' ? 'IN →' : '← OUT';
        const method = msg.method || '—';
        const msgType = msg.msg_type || 'unknown';
        const transport = (msg.transport || 'stdio').toLowerCase();
        const latency = msg.latency_ms;
        const expanded = state.expandedId === msg.id ? 'expanded' : '';

        let latencyHTML = '';
        if (latency > 0) {
            const pct = Math.min(100, (latency / 1000) * 100);
            const speedClass = latency > 5000 ? 'very-slow' : latency > 1000 ? 'slow' : '';
            latencyHTML = `
                <span class="msg-latency">
                    <span class="latency-bar"><span class="latency-fill ${speedClass}" style="width:${pct}%"></span></span>
                    ${latency}ms
                </span>`;
        }

        const sizeStr = formatBytes(msg.size_bytes || 0);
        const tokenStr = msg.token_estimate ? `${msg.token_estimate} tkn` : '';
        const errorCodeStr = msg.error_code ? `Err: ${msg.error_code}` : '';

        // Build expandable body
        const bodyParts = [];
        if (msg.params && msg.params !== 'null') {
            bodyParts.push(`<div class="json-section"><div class="json-label">params</div><div class="json-view">${syntaxHighlight(msg.params)}</div></div>`);
        }
        if (msg.result && msg.result !== 'null') {
            bodyParts.push(`<div class="json-section"><div class="json-label">result</div><div class="json-view">${syntaxHighlight(msg.result)}</div></div>`);
        }
        if (msg.error_data && msg.error_data !== 'null' && msg.error_data !== '') {
            bodyParts.push(`<div class="json-section"><div class="json-label">error</div><div class="json-view">${syntaxHighlight(msg.error_data)}</div></div>`);
        }
        if (msg.raw) {
            bodyParts.push(`<div class="json-section"><div class="json-label">raw</div><div class="json-view">${syntaxHighlight(msg.raw)}</div></div>`);
        }

        return `
            <div class="msg-card ${expanded}" data-id="${msg.id}" onclick="window.__toggleCard(${msg.id})">
                <div class="msg-header">
                    <span class="msg-direction ${dirClass}">${dirLabel}</span>
                    <span class="msg-method">${escapeHtml(method)}</span>
                    <span class="msg-type-badge ${msgType}">${msgType}</span>
                    <span class="msg-transport ${transport}">${transport}</span>
                </div>
                <div class="msg-meta">
                    <span>${time}</span>
                    ${latencyHTML}
                    <span class="msg-size">${sizeStr}</span>
                    ${tokenStr ? `<span class="msg-tokens">${tokenStr}</span>` : ''}
                    ${errorCodeStr ? `<span class="msg-error-code">${errorCodeStr}</span>` : ''}
                    ${msg.jsonrpc_id ? `<span>id: ${escapeHtml(msg.jsonrpc_id)}</span>` : ''}
                </div>
                <div class="msg-body">${bodyParts.join('')}</div>
            </div>`;
    }

    // ── Card Toggle ──
    window.__toggleCard = function (id) {
        state.expandedId = state.expandedId === id ? null : id;
        document.querySelectorAll('.msg-card').forEach((card) => {
            const cardId = parseInt(card.dataset.id);
            card.classList.toggle('expanded', cardId === state.expandedId);
        });
    };

    // ── JSON Syntax Highlighting ──
    function syntaxHighlight(raw) {
        try {
            const obj = typeof raw === 'string' ? JSON.parse(raw) : raw;
            const formatted = JSON.stringify(obj, null, 2);
            return formatted.replace(
                /("(\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+\-]?\d+)?)/g,
                (match) => {
                    let cls = 'json-number';
                    if (/^"/.test(match)) {
                        cls = /:$/.test(match) ? 'json-key' : 'json-string';
                    } else if (/true|false/.test(match)) {
                        cls = 'json-bool';
                    } else if (/null/.test(match)) {
                        cls = 'json-null';
                    }
                    return `<span class="${cls}">${escapeHtml(match)}</span>`;
                }
            );
        } catch {
            return escapeHtml(raw || '');
        }
    }

    // ── Filtering ──
    function getFilteredMessages() {
        return state.messages.filter(matchesFilter);
    }

    function matchesFilter(msg) {
        const f = state.filter;
        if (f.direction && (msg.direction || '').toUpperCase() !== f.direction) return false;
        if (f.transport && (msg.transport || '') !== f.transport) return false;
        if (f.search) {
            const s = f.search.toLowerCase();
            const hay = `${msg.method} ${msg.raw} ${msg.params} ${msg.result}`.toLowerCase();
            if (!hay.includes(s)) return false;
        }
        return true;
    }

    searchInput.addEventListener('input', (e) => {
        state.filter.search = e.target.value;
        renderTimeline();
    });

    // Filter buttons
    document.querySelectorAll('.filter-btn[data-dir]').forEach((btn) => {
        btn.addEventListener('click', () => {
            const dir = btn.dataset.dir;
            state.filter.direction = state.filter.direction === dir ? '' : dir;
            document.querySelectorAll('.filter-btn[data-dir]').forEach((b) =>
                b.classList.toggle('active', b.dataset.dir === state.filter.direction)
            );
            renderTimeline();
        });
    });

    document.querySelectorAll('.filter-btn[data-transport]').forEach((btn) => {
        btn.addEventListener('click', () => {
            const t = btn.dataset.transport;
            state.filter.transport = state.filter.transport === t ? '' : t;
            document.querySelectorAll('.filter-btn[data-transport]').forEach((b) =>
                b.classList.toggle('active', b.dataset.transport === state.filter.transport)
            );
            renderTimeline();
        });
    });

    // ── Stats ──
    async function updateStats() {
        try {
            const res = await fetch('/api/stats');
            const data = await res.json();
            statTotal.textContent = data.total_messages || 0;
            statRequests.textContent = data.total_requests || 0;
            statLatency.textContent = data.avg_latency_ms ? data.avg_latency_ms.toFixed(1) + 'ms' : '—';
            statErrors.textContent = data.total_errors || 0;
            statVolume.textContent = formatBytes(data.total_bytes || 0);
            statTokens.textContent = formatNumber(data.total_tokens || 0);
        } catch { /* ignore */ }
    }

    function updateConnectionStatus() {
        statusDot.classList.toggle('connected', state.connected);
        statusText.textContent = state.connected ? 'live' : 'disconnected';
    }

    // ── Utilities ──
    function formatTime(ts) {
        if (!ts) return '—';
        try {
            const d = new Date(ts);
            return d.toLocaleTimeString('en-US', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' })
                + '.' + String(d.getMilliseconds()).padStart(3, '0');
        } catch {
            return ts;
        }
    }

    function formatBytes(bytes) {
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
    }

    function formatNumber(num) {
        if (num >= 1000000) {
            return (num / 1000000).toFixed(1) + 'M';
        }
        if (num >= 1000) {
            return (num / 1000).toFixed(1) + 'K';
        }
        return num.toString();
    }

    function escapeHtml(str) {
        const div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    }

    // ── Init ──
    loadHistory();
    connect();
    setInterval(updateStats, 5000);
})();
