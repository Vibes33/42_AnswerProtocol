// Simulated world, only used when the page is opened without the Go relay (preview, mockup).
// It imitates the TAP server's replies line by line, in the RFC 42TAP format.
// It is never used when the client runs through `make run-client-gui` or `make run-server`.
'use strict';
window.TAPDemo = (() => {
  const ROOMS = {
    'loc.village_square': { name: 'Village Square', description: 'A bustling square with cobblestone paths. A notice board stands by the well.', exits: { north: 'loc.tavern', east: 'loc.market', west: 'loc.chapel', south: 'loc.south_gate' }, npcs: ['npc.guard', 'npc.mentor'], items: ['item.notice_board'], mobs: [] },
    'loc.tavern': { name: 'The Prancing Pony', description: 'A cozy tavern filled with warmth and laughter.', exits: { south: 'loc.village_square', west: 'loc.elder_house', southeast: 'loc.market' }, npcs: ['npc.innkeeper', 'npc.bard'], items: ['item.frothy_ale', 'item.frothy_ale'], mobs: [] },
    'loc.market': { name: 'Market Street', description: 'Stalls lined with goods and supplies.', exits: { west: 'loc.village_square', northwest: 'loc.tavern' }, npcs: ['npc.merchant'], items: [], mobs: [] },
    'loc.chapel': { name: 'Village Chapel', description: 'A quiet stone chapel smelling of incense.', exits: { east: 'loc.village_square', north: 'loc.elder_house' }, npcs: ['npc.priestess'], items: [], mobs: [] },
    'loc.elder_house': { name: "Elder's House", description: 'A cluttered home full of maps and old books.', exits: { south: 'loc.chapel', east: 'loc.tavern' }, npcs: ['npc.elder'], items: [], mobs: [] },
    'loc.south_gate': { name: 'South Gate', description: 'A wooden gate opening onto the plains.', exits: { north: 'loc.village_square', south: 'loc.green_plains' }, npcs: ['npc.gate_warden'], items: [], mobs: [] },
    'loc.green_plains': { name: 'Green Plains', description: 'Tall grass sways in the wind. Something wobbles nearby.', exits: { north: 'loc.south_gate', east: 'loc.misty_pond', south: 'loc.forest_edge' }, npcs: [], items: ['item.healing_herb', 'item.healing_herb'], mobs: ['mob.green_blob', 'mob.green_blob'] },
    'loc.misty_pond': { name: 'Misty Pond', description: 'A still pond wrapped in fog.', exits: { west: 'loc.green_plains', southwest: 'loc.forest_edge' }, npcs: [], items: ['item.healing_herb'], mobs: ['mob.blue_blob'] },
    'loc.forest_edge': { name: 'Forest Edge', description: 'The plains end at a dark line of trees. A slimy trail leads west.', exits: { north: 'loc.green_plains', northeast: 'loc.misty_pond', west: 'loc.blob_hollow' }, npcs: [], items: [], mobs: ['mob.pebble_crab', 'mob.ember_wisp', 'mob.green_blob'] },
    'loc.blob_hollow': { name: 'Blob Hollow', description: 'A damp hollow coated in slime. Something enormous breathes in the dark.', exits: { east: 'loc.forest_edge' }, npcs: [], items: [], mobs: ['mob.blob_king'] }
  };
  const NPCS = {
    'npc.guard': ['Village Guard', 'Stay safe, traveler.', 'The roads south get dangerous past the gate.'],
    'npc.mentor': ['Master Kael', 'Every hero starts somewhere. Tell me which path you walk.', 'Archer, fighter, mage or healer: each needs the others.'],
    'npc.innkeeper': ['Marta', "A bed and a hot meal? Five coins and you'll be good as new.", "Bram at the gate hasn't had a drink all week, poor man."],
    'npc.bard': ['Tomlin the Bard', 'They say a king of slime rules the hollow west of the forest edge.', 'Fire melts ice, water drowns fire... every child knows the song.'],
    'npc.merchant': ['Oswin', 'Welcome to my shop!', 'Potions, blades, bows, staves. I buy blob jelly too.'],
    'npc.priestess': ['Sister Liora', 'The light keeps us all.', 'Our herb stores are empty. The plains past the gate are full of them.'],
    'npc.elder': ['Elder Rowan', 'Blobs have been creeping closer to the village every night.', 'Something commands them. Something big.'],
    'npc.gate_warden': ['Gate Warden Bram', 'Beyond this gate lie the Verdant Plains. Mind the blobs.', "I'd kill for a cold ale."]
  };
  const MOBS = {
    'mob.green_blob': ['Green Blob', 35, 8], 'mob.blue_blob': ['Blue Blob', 40, 7], 'mob.pebble_crab': ['Pebble Crab', 50, 12],
    'mob.ember_wisp': ['Ember Wisp', 30, 9], 'mob.blob_king': ['Blob King', 220, 16]
  };
  const QUESTS = {
    'quest.herb_gathering': { giver: 'npc.priestess', kind: 'fetch', target: 'item.healing_herb', count: 3, description: 'Bring 3 Healing Herbs to Sister Liora.', reward: '40 xp, 15 gold, 2x Potion' },
    'quest.ale_delivery': { giver: 'npc.innkeeper', turnIn: 'npc.gate_warden', kind: 'deliver', target: 'item.frothy_ale', count: 1, description: 'Deliver a Frothy Ale to Gate Warden Bram.', reward: '20 xp, 10 gold' },
    'quest.blob_menace': { giver: 'npc.elder', kind: 'defeat', target: 'mob.green_blob', count: 3, description: 'Defeat 3 Green Blobs in the Verdant Plains.', reward: '60 xp, 20 gold, 1x Leather Armor' }
  };
  const OTHERS = { neo: 'loc.village_square', lune: 'loc.green_plains' };

  function create(onLine, onClose) {
    const rooms = JSON.parse(JSON.stringify(ROOMS));
    const hp = {}; // monster HP, per room and index
    const st = { name: '', room: 'loc.village_square', hp: 100, mana: 30, xp: 0, level: 1, gold: 12, inv: ['item.minor_potion', 'item.wooden_sword'], quests: {}, kills: {}, talk: {} };
    const out = (line, delay = 60) => setTimeout(() => onLine(line), delay);
    const ok = (v) => out('OK ' + (typeof v === 'string' ? v : JSON.stringify(v)));
    const norm = (s) => s.trim().toLowerCase().replace(/ /g, '_');
    const match = (q, id, name) => { q = norm(q); return q === id || q === id.split('.')[1] || (name && q === norm(name)); };
    const roomPlayers = () => [st.name, ...Object.keys(OTHERS).filter((p) => OTHERS[p] === st.room)];
    const progress = (id) => {
      const q = QUESTS[id];
      const n = q.kind === 'defeat' ? (st.kills[q.target] || 0) : st.inv.filter((x) => x === q.target).length;
      return Math.min(n, q.count);
    };

    out('OK hello proto=1', 200);

    const handlers = {
      CHARACTERS() {
        ok([
          { id: 'char.pyra', name: 'Pyra', description: 'A sharp-eyed archer whose arrows burst into flame.', class: 'Archer', class_description: 'Fast ranged fighter.', element: 'fire', stats: { hp: 100, mana: 40, attack: 14, defense: 9, magic_attack: 8, speed: 15 } },
          { id: 'char.borak', name: 'Borak', description: 'A mountain-born brawler with skin as hard as granite.', class: 'Fighter', class_description: 'Sturdy melee warrior.', element: 'rock', stats: { hp: 100, mana: 30, attack: 16, defense: 14, magic_attack: 5, speed: 9 }, default: true },
          { id: 'char.nivel', name: 'Nivel', description: 'A quiet scholar of the frozen arts.', class: 'Mage', class_description: 'Elemental caster.', element: 'ice', stats: { hp: 100, mana: 80, attack: 6, defense: 7, magic_attack: 18, speed: 11 } },
          { id: 'char.sylve', name: 'Sylve', description: 'A gentle druid who speaks with the forest.', class: 'Healer', class_description: 'Support caster.', element: 'plant', stats: { hp: 100, mana: 70, attack: 7, defense: 10, magic_attack: 12, speed: 10 } }
        ]);
      },
      CONNECT(args) {
        const [name] = args.trim().split(' '); // "CONNECT <name> [<character>]"
        st.name = name;
        ok('connected');
        out('EVT STATS players=3', 120);
        out('EVT GLOBAL CHAT lune anyone up for the Blob King tonight?', 9000);
        out('EVT GROUP INVITE lune', 16000);
      },
      LOOK() {
        const r = rooms[st.room];
        const mobs = r.mobs.filter((id, i) => (hp[st.room + i] ?? MOBS[id][1]) > 0);
        ok({ room: { id: st.room, name: r.name, description: r.description, exits: r.exits }, players: roomPlayers(), items: r.items, npcs: [...r.npcs, ...mobs] });
      },
      MOVE(dir) {
        const dest = rooms[st.room].exits[dir.trim().toLowerCase()];
        if (!dest) return out('ERR 404 EXIT_NOT_FOUND');
        st.room = dest;
        ok(rooms[dest].name);
      },
      TAKE(q) {
        const r = rooms[st.room], i = r.items.findIndex((id) => match(q, id));
        if (i < 0) return out('ERR 404 ITEM_NOT_FOUND');
        if (r.items[i] === 'item.notice_board') return out('ERR 404 ITEM_NOT_TAKEABLE');
        st.inv.push(r.items.splice(i, 1)[0]);
        ok('taken ' + st.inv[st.inv.length - 1]);
      },
      DROP(q) {
        const i = st.inv.findIndex((id) => match(q, id));
        if (i < 0) return out('ERR 404 ITEM_NOT_FOUND');
        rooms[st.room].items.push(st.inv.splice(i, 1)[0]);
        ok('dropped');
      },
      INVENTORY() { ok(st.inv); },
      TALK(q) {
        const id = rooms[st.room].npcs.find((n) => match(q, n, NPCS[n][0]));
        if (id) { const i = st.talk[id] || 0; st.talk[id] = i + 1; return ok(NPCS[id][1 + (i % 2)]); }
        if (rooms[st.room].mobs.some((m) => m === 'mob.blob_king' && match(q, m))) return ok('WHO DARES ENTER MY HOLLOW?');
        out('ERR 404 NPC_NOT_FOUND');
      },
      QUEST(q) {
        const npc = rooms[st.room].npcs.find((n) => match(q, n, NPCS[n][0]));
        if (!npc) return out('ERR 404 NPC_NOT_FOUND');
        for (const [id, p] of Object.entries(st.quests)) {
          const qd = QUESTS[id];
          if (p.done || (qd.turnIn || qd.giver) !== npc || progress(id) < qd.count) continue;
          if (qd.kind !== 'defeat') for (let k = 0; k < qd.count; k++) st.inv.splice(st.inv.indexOf(qd.target), 1);
          p.done = true; st.xp += 40; st.gold += 15;
          return ok({ quest_id: id, description: qd.description, reward: qd.reward, status: 'completed', progress: `${qd.count}/${qd.count}` });
        }
        const id = Object.keys(QUESTS).find((k) => QUESTS[k].giver === npc && !st.quests[k]);
        if (!id) return out('ERR 406 NO_QUEST_AVAILABLE');
        st.quests[id] = { done: false };
        ok({ quest_id: id, description: QUESTS[id].description, reward: QUESTS[id].reward, status: 'active', progress: `${progress(id)}/${QUESTS[id].count}` });
      },
      QUESTS() {
        ok(Object.entries(st.quests).map(([id, p]) => ({ quest_id: id, status: p.done ? 'completed' : 'active', progress: `${p.done ? QUESTS[id].count : progress(id)}/${QUESTS[id].count}` })));
      },
      ATTACK(q) {
        const room = st.room, r = rooms[room];
        const i = r.mobs.findIndex((m, k) => match(q, m, MOBS[m][0]) && (hp[room + k] ?? MOBS[m][1]) > 0);
        if (i < 0) return out(r.npcs.some((n) => match(q, n, NPCS[n][0])) ? 'ERR 405 NPC_NOT_HOSTILE' : 'ERR 404 NPC_NOT_FOUND');
        const id = r.mobs[i], [name, max, atk] = MOBS[id];
        const dmg = 8 + Math.floor(Math.random() * 9);
        hp[room + i] = Math.max(0, (hp[room + i] ?? max) - dmg);
        const log = [`${st.name} hit ${name} for ${dmg}`];
        let status = 'combat';
        if (hp[room + i] === 0) {
          status = 'victory';
          st.xp += 12; st.gold += 3; st.kills[id] = (st.kills[id] || 0) + 1;
          r.items.push('item.blob_jelly');
          log.push(`${name} was defeated`, `${name} dropped Blob Jelly`);
          setTimeout(() => { hp[room + i] = max; if (st.room === room) onLine('EVT ROOM SPAWN ' + id); }, 25000);
        } else {
          const back = 2 + Math.floor(Math.random() * atk);
          st.hp -= back;
          log.push(`${name} hit ${st.name} for ${back}`);
          if (st.hp <= 0) {
            status = 'defeated';
            log.push(`${st.name} was defeated by ${name}`);
            st.hp = 50; st.room = 'loc.village_square';
          }
        }
        ok({ attacker_hp: st.hp, target_hp: hp[room + i], damage: dmg, status, target: id, log });
      },
      STATUS() { ok({ hp: st.hp, max_hp: 100, status: st.hp < 100 ? 'wounded' : 'healthy', mana: st.mana, max_mana: 30, level: st.level, xp: st.xp, xp_next: 120, gold: st.gold, room: st.room }); },
      WHO() { ok('players=3'); },
      CHAT(rest) {
        const [scope, ...msg] = rest.split(' ');
        ok('');
        out(`EVT ${scope.toUpperCase()} CHAT ${st.name} ${msg.join(' ')}`, 90);
      },
      GROUP(rest) {
        const [sub, who] = rest.split(' ');
        switch ((sub || '').toUpperCase()) {
          case 'CREATE': ok('group=g-' + st.name.toLowerCase()); break;
          case 'INVITE':
            if (!OTHERS[who]) return out('ERR 404 PLAYER_NOT_FOUND');
            ok('invited ' + who);
            out('EVT GROUP JOIN ' + who, 1500);
            break;
          case 'JOIN': ok('group=g-' + (who || '').toLowerCase()); break;
          case 'LEAVE': ok(''); break;
          default: out('ERR 400 MISSING_ARGUMENT');
        }
      },
      QUIT() { ok('bye'); setTimeout(onClose, 250); }
    };

    return {
      send(line) {
        const sp = line.indexOf(' ');
        const verb = (sp < 0 ? line : line.slice(0, sp)).toUpperCase();
        const args = sp < 0 ? '' : line.slice(sp + 1);
        if (!st.name && verb !== 'CONNECT' && verb !== 'CHARACTERS') return out('ERR 202 NOT_CONNECTED');
        const h = handlers[verb];
        if (!h) return out('ERR 400 UNKNOWN_COMMAND');
        h(args);
      }
    };
  }

  return { create };
})();
