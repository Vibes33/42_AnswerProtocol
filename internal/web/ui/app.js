// TAP web client. The whole RFC 42TAP protocol is handled here; the Go relay only carries
// lines. Rules followed:
//  - the server answers commands in the order they were sent: each OK/ERR line answers the
//    oldest pending command (S.pending); EVT lines never enter that queue;
//  - the first line received is the greeting ("OK hello proto=1");
//  - sends are serialised (one HTTP request at a time) to keep that order.
'use strict';
(() => {
  const P = 168; // board grid step (136 px tile + gap)
  const DIRS = { north: [0, -1], south: [0, 1], east: [1, 0], west: [-1, 0], northeast: [1, -1], northwest: [-1, -1], southeast: [1, 1], southwest: [-1, 1] };
  const DIR_SHORT = { north: 'N', south: 'S', east: 'E', west: 'W', northeast: 'NE', northwest: 'NW', southeast: 'SE', southwest: 'SW' };
  const DIR_LABEL = { north: 'NORTH', south: 'SOUTH', east: 'EAST', west: 'WEST', northeast: 'NORTH-EAST', northwest: 'NORTH-WEST', southeast: 'SOUTH-EAST', southwest: 'SOUTH-WEST' };
  const COMPASS = ['northwest', 'north', 'northeast', 'west', null, 'east', 'southwest', 'south', 'southeast'];
  const NAV = [['inv', 'Inventory', 'I', 'INVENTORY'], ['quests', 'Quests', 'J', 'QUESTS'], ['status', 'Status', 'T', 'STATUS'], ['moves', 'Attacks', 'K', 'STATUS'], ['who', 'Players', 'P', 'WHO'], ['group', 'Group', 'G', null]];
  // Display labels for the move categories sent by STATUS.
  const CATEGORY = { physical: 'Physical', magical: 'Magical', status: 'Support' };
  const cap = (v) => (v ? String(v)[0].toUpperCase() + String(v).slice(1) : '');
  const KEYS = { ArrowUp: 'north', ArrowDown: 'south', ArrowLeft: 'west', ArrowRight: 'east', w: 'north', s: 'south', a: 'west', d: 'east', q: 'northwest', e: 'northeast', z: 'southwest', c: 'southeast' };

  const $ = (sel) => document.querySelector(sel);
  // Every text coming from the server (names, other players' chat) is escaped before display.
  const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const human = (id) => {
    let s = String(id ?? '');
    const dot = s.indexOf('.');
    if (dot > 0 && /^[a-z]+$/.test(s.slice(0, dot))) s = s.slice(dot + 1);
    return s.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
  };
  const parse = (s) => { try { return JSON.parse(s); } catch { return undefined; } };
  const same = (a, b) => String(a).toLowerCase() === String(b).toLowerCase();
  const isMob = (id) => /^(mob|monster)\./.test(id);
  const letter = (i) => String.fromCharCode(65 + (i % 26));

  const S = {
    name: '', addr: 'localhost:4242', demo: false, transport: null, greeted: false, inGame: false, quitting: false,
    pending: [], map: {}, here: null, room: null, players: [], items: [], npcs: [],
    inv: [], status: {}, quests: [], questInfo: {}, serverCount: 0, group: null, invite: null,
    target: null, sheet: null, tab: 'room', scope: 'ROOM', log: [], unread: 0, dialogue: null, mobHp: {},
    lookAsked: false, announced: false, characters: null, charId: null, wantLogin: false, attackMenu: false, offerKey: '', shop: null, party: null, tilt: localStorage.getItem('tap.tilt') !== 'flat', bounds: null
  };

  // ---------------------------------------------------------------- transport
  function realTransport(onLine, onClose, onFail) {
    const es = new EventSource('events');
    let opened = false, done = false, session = '';
    const finish = (fn, arg) => { if (done) return; done = true; es.close(); fn(arg); };
    es.onopen = () => { opened = true; };
    es.onmessage = (e) => onLine(e.data);
    // The relay serves several players: each tab receives the token of its own TCP connection.
    es.addEventListener('session', (e) => { session = e.data; });
    es.addEventListener('fail', (e) => finish(onFail, e.data));
    es.addEventListener('closed', () => finish(onClose));
    es.onerror = () => finish(opened ? onClose : onFail, 'relais injoignable');
    let chain = Promise.resolve();
    return {
      send(line) {
        chain = chain
          .then(() => fetch('send', { method: 'POST', body: line, headers: { 'X-TAP-Session': session } }))
          .then((r) => { if (!r.ok) throw new Error('HTTP ' + r.status); })
          .catch((err) => addLog('ERR', 'Send failed: ' + err.message));
      }
    };
  }

  function send(line) {
    if (!S.transport) return;
    S.pending.push(line);
    S.transport.send(line);
  }

  // ---------------------------------------------------------------- receiving
  function onLine(line) {
    if (!S.greeted) {
      S.greeted = true;
      if (line.startsWith('ERR')) { loginMsg('Connection refused: ' + line); return; }
      addLog('OK', line.replace(/^OK ?/, ''), false);
      // The connection opens as soon as the page loads: fetch the characters for the login
      // screen (an extension of our server; with other servers the picker stays hidden).
      send('CHARACTERS');
      if (S.wantLogin) login();
      return;
    }
    if (line.startsWith('EVT ')) { onEvent(line.slice(4)); return; }

    const cmd = S.pending.shift() || '';
    const sp = cmd.indexOf(' ');
    const verb = (sp < 0 ? cmd : cmd.slice(0, sp)).toUpperCase();
    const arg = sp < 0 ? '' : cmd.slice(sp + 1);
    if (line.startsWith('ERR')) { onError(verb, cmd, line.slice(4)); return; }
    onReply(verb, arg, line.replace(/^OK ?/, '').trim());
  }

  function onError(verb, cmd, rest) {
    if (verb === 'CONNECT' && !S.inGame) { loginMsg('Error: ' + rest); return; }
    if (verb === 'CHARACTERS') { S.characters = []; renderPicker(); return; } // server without character choice
    if (cmd.toUpperCase() === 'GROUP INFO') { S.party = null; render(); return; } // not (or no longer) in a group
    addLog('ERR ' + rest.split(' ')[0], `${cmd} → ${rest.split(' ').slice(1).join(' ') || rest}`);
  }

  function onReply(verb, arg, data) {
    switch (verb) {
      case 'CONNECT': enterGame(); break;
      case 'CHARACTERS': { const v = parse(data); S.characters = Array.isArray(v) ? v : []; renderPicker(); break; }
      case 'LOOK': applyLook(parse(data)); break;
      case 'MOVE':
        S.dialogue = null; S.target = null;
        addLog('MOVE ' + arg.toUpperCase(), data || 'Moved ' + arg);
        send('LOOK');
        break;
      case 'USE': {
        const v = parse(data);
        if (v && v.item) {
          const done = { used: 'Used', equipped: 'Equipped', unequipped: 'Unequipped' }[v.action] || 'Used';
          addLog('USE', `${done} ${human(v.item)}${v.log && v.log.length ? ' — ' + v.log.join(', ') : ''}`);
          Object.assign(S.status, { hp: v.hp, max_hp: v.max_hp, mana: v.mana, max_mana: v.max_mana });
        } else addLog('USE', data || 'OK');
        send('INVENTORY'); send('STATUS');
        break;
      }
      case 'TAKE': case 'DROP':
        addLog(verb, data || `${verb} ${human(arg)}`);
        S.target = null;
        send('LOOK'); send('INVENTORY'); send('STATUS');
        break;
      case 'INVENTORY': {
        const v = parse(data);
        S.inv = Array.isArray(v) ? v.map((x) => (typeof x === 'string' ? x : x.id || x.name || JSON.stringify(x))) : [];
        validateTarget(); render();
        break;
      }
      case 'TALK': {
        let npc = arg, text = data;
        const v = parse(data);
        if (v && v.dialogue) { npc = v.npc || npc; text = v.dialogue; } // format JSON du sujet
        S.dialogue = { name: human(npc), text };
        S.tab = 'room';
        addLog('TALK', `${human(npc)}: "${text}"`);
        render();
        break;
      }
      case 'ATTACK': applyAttack(arg, parse(data), data); send('STATUS'); send('LOOK'); send('QUESTS'); if (S.group) send('GROUP INFO'); break;
      case 'SHOP': { const v = parse(data); if (v && v.wares) { S.shop = v; S.sheet = 'shop'; } render(); break; }
      case 'BUY': case 'SELL': {
        const item = (/(?:bought|sold)=(\S+)/.exec(data) || [])[1] || arg;
        addLog(verb, `${human(item)} — ${(/gold=(\d+)/.exec(data) || [])[1] ?? '?'} gold left`);
        send('SHOP'); send('INVENTORY'); send('STATUS');
        break;
      }
      case 'STATUS': {
        const v = parse(data);
        if (v && typeof v === 'object') S.status = v;
        // A new attack unlocked while the three slots are taken: open the choice once per offer.
        const offers = (S.status.move_offers || []).join(' ');
        if (offers !== S.offerKey) { S.offerKey = offers; if (offers) S.sheet = 'learn'; }
        if (!offers && S.sheet === 'learn') S.sheet = null;
        // On the first reply, announce the character being played (fields added by our server).
        if (v && v.character && !S.announced) {
          S.announced = true;
          addLog('CHARACTER', `You play ${v.character}, a ${String(v.element || '').toLowerCase()} ${String(v.class || '').toLowerCase()}.`);
        }
        render();
        break;
      }
      case 'QUEST': {
        const v = parse(data);
        if (v && v.quest_id) {
          S.questInfo[v.quest_id] = v;
          addLog(v.status === 'completed' ? 'QUEST ✓' : 'QUEST', `${human(v.quest_id)} — ${v.description || ''}`);
        } else addLog('QUEST', data);
        send('QUESTS'); send('INVENTORY'); send('STATUS');
        break;
      }
      case 'QUESTS': { const v = parse(data); S.quests = Array.isArray(v) ? v : []; render(); break; }
      case 'WHO': {
        const m = /players=(\d+)/.exec(data);
        if (m) S.serverCount = +m[1];
        else { const v = parse(data); if (v && v.server != null) S.serverCount = v.server; }
        render();
        break;
      }
      case 'GROUP': applyGroup(arg, data); break;
      case 'LEARN': {
        const [id, how] = arg.split(' ');
        const forgot = (/forgot=(\S+)/.exec(data) || [])[1];
        addLog('LEARN', how === 'skip' ? `${moveName(id)} skipped` : `${moveName(id)} learned${forgot ? ' in place of ' + moveName(forgot) : ''}`);
        send('STATUS');
        break;
      }
      case 'QUIT': S.quitting = true; break; // the server then closes: onClosed shows the screen
      case 'CHAT': break; // the message comes back as EVT … CHAT
      default: addLog(verb || 'OK', data);
    }
  }

  function onEvent(evt) {
    const p = evt.split(' ');
    const cat = p[0], kind = p[1] || '';
    if (kind === 'CHAT') {
      const author = p[2] || '';
      addLog('CHAT ' + cat, `${author}: ${p.slice(3).join(' ')}`, !same(author, S.name));
      return;
    }
    if (cat === 'STATS') { const m = /players=(\d+)/.exec(evt); if (m) S.serverCount = +m[1]; render(); return; }
    if (cat === 'GROUP') {
      const who = p[2] || '';
      if (kind === 'INVITE') { S.invite = who; addLog('GROUP INVITE', `${who} invites you to their group.`); }
      else if (kind === 'JOIN') {
        if (S.group && !S.group.members.some((x) => same(x, who))) S.group.members.push(who);
        addLog('GROUP JOIN', `${who} joined the group.`);
      } else if (kind === 'LEAVE') {
        if (same(who, S.name)) S.group = null;
        else if (S.group) S.group.members = S.group.members.filter((x) => !same(x, who));
        addLog('GROUP LEAVE', `${who} left the group.`);
      } else if (kind === 'LEADER') {
        if (S.group) S.group.leader = who;
        addLog('GROUP', `${who} is the new leader.`);
      } else if (kind === 'MOVE') { // an ally changed room: update their tile
        const m = S.party && S.party.members.find((x) => same(x.name, who));
        if (m) { m.room = p[3]; m.room_name = S.map[p[3]] ? S.map[p[3]].name : human(p[3]); }
      } else if (kind === 'XP') { // XP share of a monster defeated by an ally in the same room
        addLog('GROUP XP', `+${p[4]} XP — ${human(p[3])} defeated by ${who}`);
        send('STATUS');
      } else addLog('EVT GROUP', p.slice(1).join(' '));
      if (S.group && kind !== 'MOVE' && !(kind === 'LEAVE' && same(who, S.name))) send('GROUP INFO');
      if (kind === 'LEAVE' && same(who, S.name)) S.party = null;
      render();
      return;
    }
    if (cat === 'ROOM') { addLog('EVT ROOM', describeRoomEvent(p)); send('LOOK'); return; }
    addLog('EVT ' + cat, p.slice(1).join(' '));
  }

  function describeRoomEvent(p) {
    const [, kind, a, b, c] = p;
    if (kind === 'PRESENCE') return `${b} ${a === 'ENTER' ? 'enters' : 'leaves'} the room.`;
    if (kind === 'ITEM') return `${b} ${a === 'TAKEN' ? 'takes' : 'drops'} ${human(c)}.`;
    if (kind === 'COMBAT') return `${a} hits ${human(b)} (−${c}).`;
    if (kind === 'SPAWN') return `${human(a)} respawns.`;
    return p.slice(1).join(' ');
  }

  // ---------------------------------------------------------------- game state
  function applyLook(v) {
    if (!v || !v.room) { addLog('LOOK', 'Unreadable reply'); return; }
    const r = v.room;
    if (!S.map[r.id]) place(r.id, null); // first room, or teleport (respawn)
    const node = S.map[r.id];
    Object.assign(node, { name: r.name || human(r.id), desc: r.description || '', visited: true, exits: r.exits || {} });
    for (const [dir, dest] of Object.entries(node.exits)) {
      const vec = DIRS[dir.toLowerCase()];
      place(dest, vec ? [node.pos[0] + vec[0], node.pos[1] + vec[1]] : node.pos);
    }
    S.here = r.id; S.room = node;
    S.players = (v.players || []).filter((n) => !same(n, S.name));
    S.items = v.items || [];
    S.npcs = v.npcs || [];
    node.counts = { npc: S.npcs.filter((x) => !isMob(x)).length, mob: S.npcs.filter(isMob).length };
    if (S.lookAsked) { S.lookAsked = false; addLog('LOOK', `${node.name} — ${node.desc}`); }
    validateTarget();
    render();
  }

  // The protocol does not send a map: it is built while exploring. Each exit is placed on the
  // neighbouring tile in its direction; on a collision, on the nearest free tile. This works
  // with any RFC server.
  function place(id, want) {
    if (S.map[id]) return;
    const taken = (x, y) => Object.values(S.map).some((n) => n.pos[0] === x && n.pos[1] === y);
    let [x, y] = want || [0, 0];
    if (taken(x, y)) {
      search: for (let r = 1; r < 8; r++) for (let dy = -r; dy <= r; dy++) for (let dx = -r; dx <= r; dx++) {
        if (Math.max(Math.abs(dx), Math.abs(dy)) !== r || taken(x + dx, y + dy)) continue;
        x += dx; y += dy; break search;
      }
    }
    S.map[id] = { pos: [x, y], name: human(id), desc: '', visited: false, exits: {}, counts: null };
  }

  function applyAttack(arg, v, raw) {
    if (!v) { addLog('ATTACK', raw); return; }
    const id = v.target || arg;
    const prev = S.mobHp[id];
    S.mobHp[id] = { hp: v.target_hp, max: Math.max(prev ? prev.max : 0, (v.target_hp || 0) + (v.damage || 0)) };
    if (typeof v.attacker_hp === 'number') S.status.hp = v.attacker_hp;
    if (typeof v.mana === 'number') S.status.mana = v.mana;
    const lines = v.log && v.log.length ? v.log : [`damage ${v.damage}, target HP ${v.target_hp}`];
    lines.forEach((l, i) => addLog('COMBAT', l, i === lines.length - 1));
    if (v.status === 'victory') { delete S.mobHp[id]; S.target = null; S.attackMenu = false; addLog('VICTORY', human(id) + ' defeated.'); }
    if (v.status === 'defeated') { S.target = null; S.attackMenu = false; addLog('DEFEAT', 'You respawn in a safe place.'); }
    render();
  }

  function applyGroup(arg, data) {
    const [subRaw, who] = arg.split(' ');
    const sub = (subRaw || '').toUpperCase();
    if (sub === 'INFO') { applyParty(parse(data)); return; }
    const m = /group=(\S+)/.exec(data);
    if (sub === 'CREATE') S.group = { id: m ? m[1] : 'groupe', leader: S.name, members: [S.name] };
    if (sub === 'JOIN') { S.group = { id: m ? m[1] : 'groupe', leader: who, members: [who, S.name].filter(Boolean) }; S.invite = null; }
    if (sub === 'LEAVE') { S.group = null; S.party = null; }
    addLog('GROUP ' + sub, data || 'OK');
    if (S.group) send('GROUP INFO');
    render();
  }

  // GROUP INFO (an extension of our server): where the allies are and in what state.
  function applyParty(v) {
    if (!v || !Array.isArray(v.members)) return;
    S.party = v;
    S.group = { id: v.id, leader: v.leader, members: v.members.map((m) => m.name) };
    render();
  }

  function validateTarget() {
    const t = S.target;
    if (!t) return;
    const ok = { item: S.items, inv: S.inv, npc: S.npcs, mob: S.npcs, player: S.players }[t.kind];
    if (!ok || !ok.includes(t.id)) S.target = null;
  }

  function enterGame() {
    S.inGame = true;
    $('#login').hidden = true;
    addLog('CONNECT', `Connected as ${S.name}`, false);
    ['LOOK', 'INVENTORY', 'STATUS', 'QUESTS', 'WHO'].forEach(send);
    render();
  }

  function onClosed() {
    if (!S.inGame) { S.transport = null; S.greeted = false; loginMsg('The server closed the connection.'); return; }
    $('#closedKicker').textContent = S.quitting ? 'OK bye' : 'EOF';
    $('#closedTitle').textContent = S.quitting ? 'Disconnected from the server' : 'Connection lost';
    $('#closedAddr').textContent = S.addr;
    $('#closed').hidden = false;
  }
  function onFail(msg) { S.transport = null; S.greeted = false; loginMsg('Cannot reach the server: ' + msg); }

  // ---------------------------------------------------------------- log & toast
  let toastTimer = 0;
  function addLog(tag, text, toast = true) {
    const d = new Date();
    const t = `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
    S.log.push({ t, tag, text });
    if (S.log.length > 400) S.log.shift();
    if (S.tab !== 'log') S.unread++;
    if (toast) {
      $('#toastTag').textContent = tag;
      $('#toastText').textContent = text;
      $('#toastWrap').hidden = false;
      clearTimeout(toastTimer);
      toastTimer = setTimeout(() => { $('#toastWrap').hidden = true; }, 2800);
    }
    renderLog(); renderTabs();
  }
  function loginMsg(s) { $('#loginMsg').textContent = s; }

  // ---------------------------------------------------------------- login & character choice
  function openTransport() {
    S.pending = []; S.greeted = false;
    S.transport = (S.demo ? window.TAPDemo.create : realTransport)(onLine, onClosed, onFail);
  }

  // login sends "CONNECT <name> [<character>]"; the character is only added when the server
  // answered CHARACTERS, so an RFC server receives a plain CONNECT.
  function login() {
    S.wantLogin = false;
    loginMsg('Connecting to ' + S.addr + '…');
    send('CONNECT ' + S.name + (S.characters && S.characters.length && S.charId ? ' ' + S.charId : ''));
  }

  function renderPicker() {
    const box = $('#picker');
    const list = S.characters || [];
    box.hidden = !list.length;
    if (!list.length) return;
    if (!list.some((c) => c.id === S.charId)) S.charId = (list.find((c) => c.default) || list[0]).id;
    const cur = list.find((c) => c.id === S.charId);
    const classes = [...new Set(list.map((c) => c.class))];
    const st = cur.stats || {};
    box.innerHTML = `<span class="pick-h">Class</span>
      <div class="seg" style="grid-template-columns:repeat(${classes.length},1fr)">${classes.map((k) =>
        `<button type="button" class="${k === cur.class ? 'on' : ''}" data-act="pick-class" data-id="${esc(k)}">${esc(k)}</button>`).join('')}</div>
      <span class="pick-d">${esc(cur.class_description)}</span>
      <span class="pick-h">Character</span>
      <div class="list">${list.filter((c) => c.class === cur.class).map((c) =>
        `<div class="row col${c.id === cur.id ? ' sel' : ''}" data-act="pick-char" data-id="${esc(c.id)}"><div class="row-top"><span class="row-name">${esc(c.name)}</span><span class="row-meta mono">${esc(String(c.element).toUpperCase())}</span></div><span class="row-sub">${esc(c.description)}</span></div>`).join('')}</div>
      <span class="pick-s mono">HP ${st.hp} · MANA ${st.mana} · ATK ${st.attack} · DEF ${st.defense} · MAG ${st.magic_attack} · SPD ${st.speed}</span>`;
  }

  // ---------------------------------------------------------------- rendu
  function render() {
    renderBoard(); renderRail(); renderSheet(); renderBar(); renderRoom(); renderTabs(); renderLog();
  }

  // Tiles and links are persistent elements (keyed by id) so the CSS transitions (lighting,
  // camera movement) play instead of everything being rebuilt.
  const tileEls = new Map(), linkEls = new Map();
  function renderBoard() {
    const nodes = Object.entries(S.map);
    if (!nodes.length) return;
    const xs = nodes.map(([, n]) => n.pos[0]), ys = nodes.map(([, n]) => n.pos[1]);
    const minX = Math.min(...xs) - 1, maxX = Math.max(...xs) + 1, minY = Math.min(...ys) - 1, maxY = Math.max(...ys) + 1;
    const cols = maxX - minX + 1, rows = maxY - minY + 1;
    const px = (pos) => [(pos[0] - minX) * P, (pos[1] - minY) * P];
    const board = $('#board');
    board.style.width = cols * P + 'px';
    board.style.height = rows * P + 'px';

    let labels = '';
    for (let i = 0; i < cols; i++) labels += `<div class="lbl-col mono" style="left:${i * P}px">${letter(i)}</div>`;
    for (let j = 0; j < rows; j++) labels += `<div class="lbl-row mono" style="top:${j * P}px">${j + 1}</div>`;
    $('#labels').innerHTML = labels;

    const occupied = new Set(nodes.map(([, n]) => n.pos.join(',')));
    let empties = '';
    for (let y = minY; y <= maxY; y++) for (let x = minX; x <= maxX; x++) {
      if (!occupied.has(x + ',' + y)) empties += `<div class="empty" style="left:${(x - minX) * P + 16}px;top:${(y - minY) * P + 16}px"></div>`;
    }
    $('#cells').innerHTML = empties;

    const seen = new Set();
    for (const [id, n] of nodes) {
      for (const dest of Object.values(n.exits)) {
        if (!S.map[dest]) continue;
        const key = [id, dest].sort().join('|');
        if (seen.has(key)) continue;
        seen.add(key);
        let el = linkEls.get(key);
        if (!el) { el = document.createElement('div'); linkEls.set(key, el); $('#links').appendChild(el); }
        const [x1, y1] = px(n.pos).map((v) => v + P / 2), [x2, y2] = px(S.map[dest].pos).map((v) => v + P / 2);
        const lit = id === S.here || dest === S.here;
        el.className = 'link' + (lit ? ' lit' : !S.map[dest].visited || !n.visited ? ' fog' : '');
        el.style.left = x1 + 'px';
        el.style.top = y1 + 'px';
        el.style.width = Math.hypot(x2 - x1, y2 - y1) + 'px';
        el.style.transform = `rotate(${Math.atan2(y2 - y1, x2 - x1)}rad)`;
      }
    }

    const adj = new Set(S.room ? Object.values(S.room.exits) : []);
    for (const [id, n] of nodes) {
      let el = tileEls.get(id);
      if (!el) { el = document.createElement('div'); el.dataset.act = 'tile'; el.dataset.id = id; tileEls.set(id, el); $('#tiles').appendChild(el); }
      const here = id === S.here;
      const [lx, ly] = px(n.pos);
      el.style.left = lx + 16 + 'px';
      el.style.top = ly + 16 + 'px';
      el.className = 'tile' + (here ? ' here' : adj.has(id) ? ' adj' : '') + (n.visited ? '' : ' unknown');
      const tokens = here ? [`<div class="token me">${esc((S.name[0] || '?').toUpperCase())}</div>`]
        .concat(S.players.slice(0, 4).map((p) => `<div class="token">${esc(p[0].toUpperCase())}</div>`)).join('') : '';
      const c = n.counts;
      const markers = !n.visited ? 'Unexplored' : c ? [c.npc ? c.npc + ' NPC' : '', c.mob ? c.mob + ' ▲' : ''].filter(Boolean).join(' · ') : '';
      const tag = !n.visited ? '?' : c && c.mob ? 'Hostile' : '';
      el.innerHTML = `<div class="tile-top mono"><span>${letter(n.pos[0] - minX)}${n.pos[1] - minY + 1}</span><span>${tag}</span></div>`
        + `<div class="tile-name">${esc(n.name)}</div>`
        + `<div class="tile-foot"><div class="tokens">${tokens}</div><span class="markers">${markers}</span></div>`;
    }

    if (S.here) {
      const [hx, hy] = px(S.map[S.here].pos);
      board.style.transform = `translate(${-(hx + P / 2)}px, ${-(hy + P / 2)}px)`;
    }
    S.bounds = { minX, minY };
  }

  function coordOf(id) {
    const n = S.map[id];
    return n && S.bounds ? letter(n.pos[0] - S.bounds.minX) + (n.pos[1] - S.bounds.minY + 1) : '';
  }

  function renderRail() {
    const st = S.status;
    $('#avatar').textContent = (S.name[0] || '?').toUpperCase();
    $('#hudName').textContent = S.name || '—';
    $('#hudSub').textContent = [st.character, st.class, st.level != null ? 'Lv. ' + st.level : ''].filter(Boolean).join(' · ');
    const hp = st.hp ?? 0, maxHp = st.max_hp || 100;
    $('#hpLabel').textContent = st.hp != null ? `${hp} / ${maxHp}` : '— / —';
    $('#hpFill').style.width = Math.max(0, Math.min(100, hp / maxHp * 100)) + '%';
    $('#manaMeter').hidden = !st.max_mana;
    if (st.max_mana) {
      $('#manaLabel').textContent = `${st.mana} / ${st.max_mana}`;
      $('#manaFill').style.width = (st.mana / st.max_mana * 100) + '%';
    }
    $('#xpLabel').textContent = st.xp != null ? `XP ${st.xp}${st.xp_next ? ' / ' + st.xp_next : ''}` : '';
    $('#goldLabel').textContent = st.gold != null ? st.gold + ' gold' : '';
    $('#nav').innerHTML = NAV.map(([id, label, key]) => {
      const dot = (id === 'group' && S.invite) || (id === 'moves' && offeredMoves().length) ? '<span class="dot"></span>' : '';
      return `<button class="nav-btn${S.sheet === id ? ' on' : ''}" data-act="nav" data-id="${id}"><span class="lbl">${label}</span>${dot}<span class="kbd mono">${key}</span></button>`;
    }).join('');
    $('#serverLine').textContent = (S.demo ? 'DEMO · ' : '') + S.addr + (S.serverCount ? ` · ${S.serverCount} online` : '');
    $('#tiltBtn').textContent = S.tilt ? 'Flat view' : 'Tilted view';
  }

  function counted(list) {
    const m = new Map();
    list.forEach((x) => m.set(x, (m.get(x) || 0) + 1));
    return [...m.entries()];
  }

  function renderSheet() {
    const sheet = $('#sheet');
    sheet.hidden = !S.sheet;
    if (!S.sheet) return;
    const sel = (k, id) => S.target && S.target.kind === k && S.target.id === id ? ' sel' : '';
    let title = '', sub = '', body = '';
    switch (S.sheet) {
      case 'inv': {
        // With our server, STATUS describes the bag (names, usable, equipped); otherwise only ids.
        const bag = S.status.bag || counted(S.inv).map(([id, n]) => ({ id, name: human(id), count: n, usable: true }));
        title = 'Inventory'; sub = S.inv.length + ' item' + (S.inv.length > 1 ? 's' : '');
        body = bag.length
          ? `<div class="list">${bag.map((it) => {
            const label = it.equipped ? 'Unequip' : it.category === 'equipment' ? 'Equip' : 'Use';
            const btn = it.usable ? `<button class="take" data-act="use" data-id="${esc(it.id)}">${label}</button>` : '';
            return `<div class="row item${sel('inv', it.id)}" data-act="select" data-kind="inv" data-id="${esc(it.id)}"><div class="row-main"><span class="row-name">${esc(it.name)}${it.count > 1 ? ` <em>×${it.count}</em>` : ''}${it.equipped ? ' <em>equipped</em>' : ''}</span><span class="row-sub">${esc(it.description || it.id)}</span></div>${btn}</div>`;
          }).join('')}</div>
             <span class="note">Use or equip an item here, or select it and press Drop in the action bar.</span>`
          : '<span class="note">Your bag is empty. Pick up items from the Room panel.</span>';
        break;
      }
      case 'quests': {
        title = 'Quests'; sub = S.quests.filter((q) => q.status !== 'completed').length + ' active';
        body = S.quests.length ? S.quests.map((q) => {
          const info = S.questInfo[q.quest_id] || {};
          const m = /(\d+)\s*\/\s*(\d+)/.exec(q.progress || '');
          const pct = q.status === 'completed' ? 100 : m ? Math.round(+m[1] / +m[2] * 100) : 0;
          return `<div class="card"><div class="card-top"><span class="card-t">${esc(human(q.quest_id))}</span><span class="card-m mono">${q.status === 'completed' ? 'COMPLETED' : esc(q.progress || q.status)}</span></div>`
            + (info.description ? `<span class="card-d">${esc(info.description)}</span>` : '')
            + `<div class="track"><div class="fill" style="width:${pct}%"></div></div>`
            + (info.reward ? `<span class="card-m">Reward: ${esc(info.reward)}</span>` : '') + '</div>';
        }).join('') : '<span class="note">No quests. Target a character and use Quest.</span>';
        break;
      }
      case 'status': {
        const st = S.status;
        title = 'Status'; sub = st.room ? human(st.room) : '';
        const cells = [['Character', st.character], ['Class', st.class], ['Element', cap(st.element)], ['HP', st.hp != null ? `${st.hp}/${st.max_hp}` : null], ['Mana', st.max_mana ? `${st.mana}/${st.max_mana}` : null], ['Level', st.level], ['XP', st.xp != null ? `${st.xp}${st.xp_next ? '/' + st.xp_next : ''}` : null], ['Gold', st.gold], ['State', st.status]]
          .filter(([, v]) => v != null && v !== '');
        body = `<div class="stats">${cells.map(([l, v]) => `<div class="stat"><small>${l}</small><b class="mono">${esc(v)}</b></div>`).join('')}</div>`;
        break;
      }
      case 'moves': {
        const st = S.status, moves = st.moves || [];
        const slots = equippedMoves(), offers = offeredMoves();
        const upcoming = moves.filter((m) => !m.learned);
        const forgotten = st.equipped_moves ? moves.filter((m) => m.learned && !m.equipped && !m.offered) : [];
        title = 'Attacks';
        sub = [st.character, st.class, cap(st.element)].filter(Boolean).join(' · ');
        if (!moves.length) { body = '<span class="note">This server does not send the list of attacks.</span>'; break; }
        if (offers.length) body += `<div class="card inverse"><div class="card-top"><span class="card-t">${esc(offers[0].name)}</span><span class="card-m mono">NEW</span></div><span class="card-d">You can learn a new attack.</span><button class="btn-primary sm" style="background:#000;color:#fff" data-act="nav" data-id="learn">Choose</button></div>`;
        body += `<span class="pick-h">Equipped · ${slots.length}/${st.move_slots || 3}</span>` + slots.map((m) => moveCard(m, 'EQUIPPED')).join('');
        if (upcoming.length) body += '<span class="pick-h">Upcoming</span>' + upcoming.map((m) => moveCard(m, 'LV. ' + m.level, true)).join('');
        if (forgotten.length) body += '<span class="pick-h">Forgotten</span>' + forgotten.map((m) => moveCard(m, 'FORGOTTEN', true)).join('');
        body += `<span class="note">You carry ${st.move_slots || 3} attacks at most. A new one replaces an equipped attack, or is skipped.</span>`;
        break;
      }
      case 'learn': {
        const offer = offeredMoves()[0];
        if (!offer) { S.sheet = null; return renderSheet(); }
        const slots = equippedMoves(), max = S.status.move_slots || 3, full = slots.length >= max;
        const left = offeredMoves().length;
        title = 'New attack'; sub = `Level ${S.status.level ?? '?'}${left > 1 ? ` · ${left} to choose` : ''}`;
        body = `<div class="card inverse"><div class="card-top"><span class="card-t">${esc(offer.name)}</span><span class="card-m mono">NEW</span></div><span class="card-d">${esc(offer.description)}</span><span class="card-m mono">${esc(moveInfo(offer))}</span></div>`;
        body += full
          ? `<span class="pick-h">Forget an attack to learn ${esc(offer.name)}</span><div class="list">${slots.map((m) =>
            `<div class="row item static"><div class="row-main"><span class="row-name">${esc(m.name)}</span><span class="row-sub mono">${esc(moveInfo(m))}</span></div><button class="take" data-act="learn" data-id="${esc(offer.id)}" data-cmd="${esc(m.id)}">Replace</button></div>`).join('')}</div>`
          : `<button class="btn-primary sm" data-act="learn" data-id="${esc(offer.id)}">Learn ${esc(offer.name)}</button>`;
        body += `<button class="btn-ghost sm" data-act="learn" data-id="${esc(offer.id)}" data-cmd="skip">Skip ${esc(offer.name)}</button>
          <span class="note">A forgotten or skipped attack cannot be learned again.</span>`;
        break;
      }
      case 'who': {
        title = 'Players'; sub = 'WHO command';
        body = `<div class="card"><span class="card-m">Connected to the server</span><span class="big-num mono">${S.serverCount}</span></div>
          <div class="list">${[S.name + ' (you)', ...S.players].map((p) => `<div class="row static"><div class="ring">${esc(p[0].toUpperCase())}</div><div class="row-main"><span class="row-name">${esc(p)}</span><span class="row-sub">${esc(S.room ? S.room.name : '')}</span></div></div>`).join('')}</div>`;
        break;
      }
      case 'group': {
        title = 'Group'; sub = S.group ? `${S.group.members.length} member${S.group.members.length > 1 ? 's' : ''} · ${S.group.id}` : 'No group';
        if (S.invite) body += `<div class="card inverse"><span class="card-t">${esc(S.invite)} invites you</span><span class="card-d">Join their group?</span><div class="two"><button class="btn-ghost sm" style="color:#000;border-color:rgba(0,0,0,.2)" data-act="ignore">Ignore</button><button class="btn-primary sm" style="background:#000;color:#fff" data-act="join">Join</button></div></div>`;
        if (S.group) body += `<div class="list">${S.group.members.map((m) => `<div class="row static"><span class="row-name" style="flex:1">${esc(m)}</span><span class="row-meta">${same(m, S.group.leader) ? 'Leader' : 'Member'}</span></div>`).join('')}</div>`;
        body += `<div class="two">${S.group
          ? `<button class="btn-ghost sm" data-act="group" data-id="LEAVE">Leave</button><button class="btn-primary sm" data-act="action" data-cmd="GROUP INVITE"${S.target && S.target.kind === 'player' ? '' : ' disabled style="opacity:.35"'}>Invite target</button>`
          : '<button class="btn-primary sm" data-act="group" data-id="CREATE" style="grid-column:span 2">Create a group</button>'}</div>
          <span class="note">To invite someone, select a player in the Room panel.</span>`;
        break;
      }
      case 'shop': {
        const sh = S.shop || { wares: [], buyback: [] };
        const gold = S.status.gold ?? sh.gold;
        const row = (it, act, label, enabled) => `<div class="row item static"><div class="row-main"><span class="row-name">${esc(it.name)}${it.count > 1 ? ` <em>×${it.count}</em>` : ''}</span><span class="row-sub">${esc(it.description)}</span><span class="row-sub mono">${it.price} gold${it.level_required ? ' · lv. ' + it.level_required : ''}</span></div><button class="take" data-act="${act}" data-id="${esc(it.id)}"${enabled ? '' : ' disabled style="opacity:.35"'}>${label}</button></div>`;
        title = sh.name || 'Merchant'; sub = `${gold} gold · SHOP / BUY / SELL`;
        body = `<span class="pick-h">For sale</span><div class="list">${sh.wares.map((it) => row(it, 'buy', 'Buy', gold >= it.price)).join('')}</div>`
          + `<span class="pick-h">Buys from you</span>`
          + (sh.buyback.length ? `<div class="list">${sh.buyback.map((it) => row(it, 'sell', 'Sell', true)).join('')}</div>` : '<span class="note">Nothing in your bag to sell.</span>');
        break;
      }
      case 'quit':
        title = 'Quit the game?'; sub = 'QUIT';
        body = `<span class="card-d" style="font-size:15px">Your items will be dropped in the current room. The other players will receive a leave event.</span>
          <div class="two"><button class="btn-ghost sm" data-act="close">Cancel</button><button class="btn-primary sm" data-act="doquit">Quit</button></div>`;
        break;
    }
    $('#sheetTitle').textContent = title;
    $('#sheetSub').textContent = sub;
    $('#sheetBody').innerHTML = body;
  }

  function renderBar() {
    const exits = S.room ? S.room.exits : {};
    $('#compass').innerHTML = COMPASS.map((d) => {
      if (!d) return '<button class="cp look mono" data-act="look" title="Look (L)">LOOK</button>';
      const dest = exits[d];
      return `<button class="cp mono${dest ? ' on' : ''}" ${dest ? `data-act="move" data-dir="${d}"` : 'disabled'} title="${DIR_LABEL[d]}${dest ? ' → ' + esc(S.map[dest] ? S.map[dest].name : human(dest)) : ''}">${DIR_SHORT[d]}</button>`;
    }).join('');

    const t = S.target;
    const kindLabel = { mob: 'monster', npc: 'NPC', inv: 'inventory', player: 'player', item: 'on the ground' };
    $('#target').innerHTML = '<span class="target-lbl">Target</span>' + (t
      ? `<div class="chip"><span>${esc(human(t.id))} · ${kindLabel[t.kind]}</span><button data-act="clear" title="Clear (Esc)">×</button></div>`
      : '<span class="hint">Select something in the Room panel</span>');
    $('#roomId').textContent = S.here || '';

    const k = t && t.kind;
    if (k !== 'mob') S.attackMenu = false;
    $('#actions').classList.toggle('moves', S.attackMenu);
    if (S.attackMenu) { renderMoves(); return; }
    const actions = [
      ['Take', 'TAKE', k === 'item', k === 'item'],
      ['Use', 'USE', k === 'inv' && invUsable(t.id), k === 'inv'],
      ['Drop', 'DROP', k === 'inv', false],
      ['Talk', 'TALK', k === 'npc' || k === 'mob', k === 'npc'],
      ['Quest', 'QUEST', k === 'npc', false],
      ['Trade', 'SHOP', k === 'npc', false],
      ['Attack', 'ATTACK', k === 'mob', k === 'mob'],
      ['Invite', 'GROUP INVITE', k === 'player' && !!S.group, k === 'player'],
      ['Join', 'GROUP JOIN', !!S.invite, !!S.invite],
      S.group ? ['Leave group', 'GROUP LEAVE', true, false] : ['Create group', 'GROUP CREATE', true, false]
    ];
    $('#actions').innerHTML = actions.map(([label, cmd, on, primary]) =>
      `<button class="ac${on ? ' on' : ''}${on && primary ? ' primary' : ''}" ${on ? `data-act="action" data-cmd="${cmd}"` : 'disabled'}><span>${label}</span><small>${cmd}</small></button>`).join('');
  }

  // An inventory item can be used unless STATUS says otherwise (materials, fixtures).
  function invUsable(id) {
    const it = (S.status.bag || []).find((b) => b.id === id);
    return it ? it.usable : true;
  }

  // The attacks carried into combat, in slot order. A server without slots (no
  // equipped_moves field) gets the first three learned attacks.
  function equippedMoves() {
    const moves = S.status.moves || [];
    if (!S.status.equipped_moves) return moves.filter((m) => m.learned).slice(0, 3);
    return S.status.equipped_moves.map((id) => moves.find((m) => m.id === id)).filter(Boolean);
  }
  function offeredMoves() {
    const moves = S.status.moves || [];
    return (S.status.move_offers || []).map((id) => moves.find((m) => m.id === id)).filter(Boolean);
  }
  function moveName(id) {
    const m = (S.status.moves || []).find((x) => x.id === id);
    return m ? m.name : human(id);
  }
  function moveInfo(m) {
    return [String(m.type).toUpperCase(), (CATEGORY[m.category] || m.category).toUpperCase(), m.category === 'status' ? '' : 'POW ' + m.power, m.mana_cost + ' MP'].filter(Boolean).join(' · ');
  }
  function moveCard(m, tag, dim) {
    return `<div class="card"${dim ? ' style="opacity:.45"' : ''}><div class="card-top"><span class="card-t">${esc(m.name)}</span><span class="card-m mono">${tag}</span></div>`
      + `<span class="card-d">${esc(m.description)}</span><span class="card-m mono">${esc(moveInfo(m))}</span></div>`;
  }

  // Pokémon-style attack menu: the bar becomes three large attack slots and a Back button.
  function renderMoves() {
    const mana = S.status.mana ?? 0;
    const moves = equippedMoves();
    const max = S.status.move_slots || 3;
    let h = '';
    for (let i = 0; i < max; i++) {
      const m = moves[i];
      if (!m) { h += '<button class="mv empty" disabled><span class="mv-name">Empty slot</span></button>'; continue; }
      const on = mana >= m.mana_cost;
      h += `<button class="mv${on ? ' on' : ''}${on && m.category !== 'status' ? ' primary' : ''}" ${on ? `data-act="use-move" data-id="${esc(m.id)}"` : 'disabled'} title="${esc(m.name + ' — ' + (CATEGORY[m.category] || m.category) + '. ' + m.description)}">`
        + `<span class="mv-top mono"><span>${esc(String(m.type).toUpperCase())}</span><span>${i + 1}</span></span>`
        + `<span class="mv-name">${esc(m.name)}</span>`
        + `<span class="mv-foot mono">${on ? (m.category === 'status' ? 'SUPPORT' : 'POW ' + m.power) + ' · ' + m.mana_cost + ' MP' : 'NO MANA · ' + m.mana_cost + ' MP'}</span></button>`;
    }
    $('#actions').innerHTML = h + '<button class="ac on" data-act="attack-back"><span>Back</span><small>ESC</small></button>';
  }

  // Fixed layout: every block keeps its place from one room to the next and the panel never
  // scrolls. The four lists sit in a 2×2 grid; only a crowded list scrolls inside its own box.
  function renderRoom() {
    const view = $('#roomView');
    view.hidden = S.tab !== 'room';
    $('#logView').hidden = S.tab !== 'log';
    if (!S.room) { view.innerHTML = ''; return; }
    const keep = [...view.querySelectorAll('.slot .list')].map((l) => l.scrollTop);
    const r = S.room;
    const sel = (k, id) => S.target && S.target.kind === k && S.target.id === id ? ' sel' : '';
    const hostile = r.counts && r.counts.mob > 0;
    const slot = (title, rows, empty) => `<div class="sec slot"><span class="sec-h">${title}${rows.length ? ` <em class="mono">${rows.length}</em>` : ''}</span>`
      + (rows.length ? `<div class="list">${rows.join('')}</div>` : `<div class="list none"><span>${empty}</span></div>`) + '</div>';

    const npcs = counted(S.npcs.filter((x) => !isMob(x))).map(([id]) =>
      `<div class="row cmp${sel('npc', id)}" data-act="select" data-kind="npc" data-id="${esc(id)}"><span class="row-name">${esc(human(id))}</span></div>`);
    const mobs = counted(S.npcs.filter(isMob)).map(([id, n]) => {
      const hp = S.mobHp[id];
      const bar = hp ? `<div class="hp"><div class="track"><div class="fill" style="width:${hp.max ? Math.round(hp.hp / hp.max * 100) : 100}%"></div></div><span class="mono">${hp.hp}/${hp.max}</span></div>` : '<span class="row-sub mono">HP ?</span>';
      return `<div class="row cmp col${sel('mob', id)}" data-act="select" data-kind="mob" data-id="${esc(id)}"><span class="row-name">${esc(human(id))}${n > 1 ? ` <em>×${n}</em>` : ''}</span>${bar}</div>`;
    });
    const items = counted(S.items).map(([id, n]) =>
      `<div class="row cmp${sel('item', id)}" data-act="select" data-kind="item" data-id="${esc(id)}" data-take="${esc(id)}" title="Double-click to take"><span class="row-name">${esc(human(id))}${n > 1 ? ` <em>×${n}</em>` : ''}</span></div>`);
    const players = S.players.map((p) =>
      `<div class="row cmp${sel('player', p)}" data-act="select" data-kind="player" data-id="${esc(p)}"><div class="ring">${esc(p[0].toUpperCase())}</div><span class="row-name">${esc(p)}</span></div>`);

    view.innerHTML = `<div class="rh"><div class="rh-top"><span>Current room · ${coordOf(S.here)}</span><span class="pill strong${hostile ? ' inv' : ''}">${hostile ? 'Hostile' : 'Calm'}</span></div>
        <span class="rh-name" title="${esc(r.name)}">${esc(r.name)}</span></div>`
      + renderParty()
      + (S.dialogue
        ? `<div class="bubble" title="${esc(S.dialogue.text)}"><b>${esc(S.dialogue.name)}</b><span>"${esc(S.dialogue.text)}"</span></div>`
        : '<div class="bubble idle"><b>Dialogue</b><span>Select a character, then Talk.</span></div>')
      + `<div class="slots">${slot('Characters', npcs, 'Nobody')}${slot('Monsters', mobs, 'None')}${slot('Items', items, 'Nothing')}${slot('Players', players, 'Alone')}</div>`;
    view.querySelectorAll('.slot .list').forEach((l, i) => { l.scrollTop = keep[i] || 0; });
    fitDialogue(view.querySelector('.bubble:not(.idle) span'));
  }

  // The bubble has a fixed height (3 lines fit every NPC of our world); a longer reply
  // shrinks its text instead of being cut, so the whole line stays readable.
  function fitDialogue(el) {
    if (!el) return;
    for (let size = 14; size > 9 && el.scrollHeight > el.clientHeight; size--) el.style.fontSize = size - 1 + 'px';
  }

  // Group map: the explored rooms in miniature, your tile in white and your allies' tiles marked
  // with their initial, then one line of allies. Fixed size, whatever the map's shape.
  function renderParty() {
    const members = S.party ? S.party.members.filter((m) => !same(m.name, S.name)) : [];
    const nodes = Object.entries(S.map);
    const xs = nodes.map(([, n]) => n.pos[0]), ys = nodes.map(([, n]) => n.pos[1]);
    const minX = Math.min(...xs), minY = Math.min(...ys);
    const cols = Math.max(...xs) - minX + 1, rows = Math.max(...ys) - minY + 1;
    const cell = Math.max(6, Math.min(26, Math.floor(296 / cols), Math.floor(92 / rows)));
    const allies = {};
    members.forEach((m) => { (allies[m.room] = allies[m.room] || []).push(m); });
    const cells = nodes.map(([id, n]) => {
      const here = id === S.here, mates = allies[id] || [];
      const cls = 'mm-cell' + (n.visited ? '' : ' fog') + (here ? ' me' : '') + (mates.length ? ' ally' : '');
      const label = cell < 14 ? '' : mates.length ? esc(mates.map((m) => m.name[0].toUpperCase()).join('')) : here ? esc((S.name[0] || '').toUpperCase()) : '';
      return `<div class="${cls}" title="${esc(n.name)}" style="left:${(n.pos[0] - minX) * cell}px;top:${(n.pos[1] - minY) * cell}px;width:${cell - 3}px;height:${cell - 3}px">${label}</div>`;
    }).join('');
    const strip = members.length
      ? members.map((m) => `<span class="ally" title="${esc(m.name)} · ${esc(m.room_name || human(m.room))}"><b>${esc(m.name[0].toUpperCase())}</b>${esc(m.name)} <i class="mono">${m.max_hp ? Math.round(m.hp / m.max_hp * 100) : 0}%</i></span>`).join('')
      : `<span class="ally-none">${S.group ? 'No ally has joined yet' : 'No group'}</span>`;
    return `<div class="mm"><div class="mm-box"><div class="mm-grid" style="width:${cols * cell}px;height:${rows * cell}px">${cells}</div></div><div class="allies">${strip}</div></div>`;
  }

  function renderTabs() {
    document.querySelectorAll('#tabs button').forEach((b) => b.classList.toggle('on', b.dataset.id === S.tab));
    $('#logTab').textContent = 'Log' + (S.unread && S.tab !== 'log' ? ` · ${S.unread}` : '');
    $('#scopes').innerHTML = [['GLOBAL', 'Global'], ['ROOM', 'Room'], ['GROUP', 'Group']].map(([k, l]) =>
      `<button type="button" class="${S.scope === k ? 'on' : ''}" data-act="scope" data-id="${k}">${l}</button>`).join('');
  }

  function renderLog() {
    if (S.tab !== 'log') return;
    $('#logList').innerHTML = S.log.slice().reverse().map((l) => {
      const cls = l.tag.startsWith('ERR') ? 'err' : l.tag.startsWith('CHAT') ? 'chat' : l.tag.startsWith('EVT') ? 'evt' : '';
      return `<div class="lg ${cls}"><time>${l.t}</time><div><small>${esc(l.tag)}</small><span>${esc(l.text)}</span></div></div>`;
    }).join('');
  }

  // ---------------------------------------------------------------- interactions
  function doAction(cmd) {
    const t = S.target;
    if (cmd === 'GROUP CREATE' || cmd === 'GROUP LEAVE') return send(cmd);
    if (cmd === 'SHOP') return send('SHOP'); // the merchant is the one in the room
    // With the list of attacks (our server), Attack opens the menu; otherwise the RFC basic attack.
    if (cmd === 'ATTACK' && t && equippedMoves().length) { S.attackMenu = true; renderBar(); return; }
    if (cmd === 'GROUP JOIN') return S.invite && send('GROUP JOIN ' + S.invite);
    if (!t) return;
    send(`${cmd} ${t.id}`);
  }

  function toggleSheet(id) {
    S.sheet = S.sheet === id ? null : id;
    const nav = NAV.find((n) => n[0] === id);
    if (S.sheet && nav && nav[3]) send(nav[3]); // refresh the content when opening
    render();
  }

  function moveTo(id) {
    if (!S.room) return;
    const dir = Object.keys(S.room.exits).find((d) => S.room.exits[d] === id);
    if (dir) send('MOVE ' + dir);
  }

  function applyTilt() {
    $('#app').classList.toggle('flat', !S.tilt);
    localStorage.setItem('tap.tilt', S.tilt ? 'tilt' : 'flat');
  }

  document.addEventListener('click', (e) => {
    const el = e.target.closest('[data-act]');
    if (!el || el.disabled) return;
    const { act, id, kind, dir, cmd } = el.dataset;
    switch (act) {
      case 'move': send('MOVE ' + dir); break;
      case 'look': S.lookAsked = true; send('LOOK'); break;
      case 'tile': moveTo(id); break;
      case 'select':
        S.target = S.target && S.target.kind === kind && S.target.id === id ? null : { kind, id };
        render();
        break;
      case 'take': send('TAKE ' + id); break;
      case 'action': doAction(cmd); break;
      case 'clear': S.target = null; render(); break;
      case 'nav': toggleSheet(id); break;
      case 'close': S.sheet = null; render(); break;
      case 'tab': S.tab = id; if (id === 'log') S.unread = 0; render(); break;
      case 'scope': S.scope = id; renderTabs(); $('#chatInput').focus(); break;
      case 'tilt': S.tilt = !S.tilt; applyTilt(); renderRail(); break;
      case 'quit': S.sheet = 'quit'; render(); break;
      case 'doquit': S.quitting = true; S.sheet = null; send('QUIT'); render(); break;
      case 'group': send('GROUP ' + id); break;
      case 'join': send('GROUP JOIN ' + S.invite); break;
      case 'ignore': S.invite = null; render(); break;
      case 'reload': location.reload(); break;
      case 'pick-class': {
        const list = S.characters || [];
        const inClass = list.filter((c) => c.class === id);
        S.charId = (inClass.find((c) => c.default) || inClass[0]).id;
        renderPicker();
        break;
      }
      case 'pick-char': S.charId = id; renderPicker(); break;
      case 'use-move': if (S.target) send(`ATTACK ${S.target.id} ${id}`); break;
      case 'learn': send(`LEARN ${id}${cmd ? ' ' + cmd : ''}`); break;
      case 'attack-back': S.attackMenu = false; renderBar(); break;
      case 'buy': send('BUY ' + id); break;
      case 'use': send('USE ' + id); break;
      case 'sell': send('SELL ' + id); break;
    }
  });

  document.addEventListener('dblclick', (e) => {
    const el = e.target.closest('[data-take]');
    if (el) send('TAKE ' + el.dataset.take);
  });

  document.addEventListener('keydown', (e) => {
    if (!S.inGame || e.metaKey || e.ctrlKey || e.altKey) return;
    const tag = document.activeElement && document.activeElement.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA') { if (e.key === 'Escape') document.activeElement.blur(); return; }
    const key = e.key.length === 1 ? e.key.toLowerCase() : e.key;
    const nav = NAV.find((n) => n[2].toLowerCase() === key);
    if (S.attackMenu && /^[1-9]$/.test(key)) { const m = equippedMoves()[+key - 1]; if (m && S.target && (S.status.mana ?? 0) >= m.mana_cost) send(`ATTACK ${S.target.id} ${m.id}`); }
    else if (KEYS[key] && S.room && S.room.exits[KEYS[key]]) { e.preventDefault(); send('MOVE ' + KEYS[key]); }
    else if (nav) toggleSheet(nav[0]);
    else if (key === 'l') { S.lookAsked = true; send('LOOK'); }
    else if (key === 'Enter') { S.tab = 'log'; S.unread = 0; render(); $('#chatInput').focus(); e.preventDefault(); }
    else if (key === 'Escape') { if (S.attackMenu) S.attackMenu = false; else { S.sheet = null; S.target = null; } render(); }
  });

  $('#chatForm').addEventListener('submit', (e) => {
    e.preventDefault();
    const input = $('#chatInput');
    const msg = input.value.trim();
    if (!msg) return;
    input.value = '';
    send(`CHAT ${S.scope} ${msg}`);
  });

  $('#loginForm').addEventListener('submit', (e) => {
    e.preventDefault();
    const name = $('#loginName').value.trim();
    if (!name) { loginMsg('Choose a name.'); return; }
    S.name = name;
    if (S.transport && S.greeted) { login(); return; } // connection already open (or name refused: try again)
    S.wantLogin = true; // CONNECT goes out right after the greeting
    loginMsg('Connecting to ' + S.addr + '…');
    if (!S.transport) openTransport();
  });

  // Served by the Go relay, /config exists. Opened without the relay (file, preview), it does
  // not: switch to the simulated world of demo.js.
  (async () => {
    applyTilt();
    try {
      const r = await fetch('config');
      const c = await r.json();
      S.addr = c.addr;
    } catch {
      S.demo = !!window.TAPDemo;
      S.addr = 'local demo';
    }
    $('#loginAddr').textContent = S.demo ? 'Demo mode — simulated world, no server' : 'Server ' + S.addr;
    if (S.demo) $('#loginName').value = 'Ada';
    $('#loginName').focus();
    renderRail(); renderBar(); renderTabs();
    openTransport();
  })();
})();
