/* HomeHub — mobile screens. Each Screen is a 402×874 surface that drops
   inside an IOSDevice. The status bar + home indicator are drawn by the
   IOS frame; we just paint our app content into the middle. */

const PAD = 22;

// ── shared bits ──────────────────────────────────────────────

const StatusBarPad = () => (<div style={{ height: 54 }} />);

const TabBar = ({ active = "home" }) => {
  const items = [
    { id: "home",      label: "Home",     d: I.home },
    { id: "rooms",     label: "Rooms",    d: I.rooms },
    { id: "scenes",    label: "Scenes",   d: I.scenes },
    { id: "schedule",  label: "Schedule", d: I.schedule },
    { id: "settings",  label: "Settings", d: I.settings },
  ];
  return (
    <div className="tabbar">
      {items.map(it => (
        <button key={it.id} className={it.id === active ? "active" : ""}>
          <Icon d={it.d} size={22} stroke={1.7}/>
          <span>{it.label}</span>
        </button>
      ))}
    </div>
  );
};

const SectionHead = ({ title, right }) => (
  <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", padding: `0 ${PAD}px`, marginTop: 26, marginBottom: 12 }}>
    <h2 style={{ fontSize: 17, fontWeight: 600 }}>{title}</h2>
    {right ?? null}
  </div>
);

const ProtocolBadge = ({ kind }) => {
  const map = {
    rf:     { color: "var(--p-rf)",     d: I.rf,     label: "RF" },
    wifi:   { color: "var(--p-wifi)",   d: I.wifi,   label: "Wi-Fi" },
    matter: { color: "var(--p-matter)", d: I.matter, label: "Matter" },
    mqtt:   { color: "var(--p-mqtt)",   d: I.matter, label: "MQTT" },
  };
  const c = map[kind] || map.rf;
  return (
    <span style={{ display: "inline-flex", alignItems: "center", gap: 4, color: c.color, fontFamily: "var(--font-mono)", fontSize: 10, letterSpacing: "0.04em", textTransform: "uppercase" }}>
      <Icon d={c.d} size={11} stroke={2}/>
      {c.label}
    </span>
  );
};

// generic device tile (used in home + room view)
const DeviceTile = ({ name, room, on, dim, protocol = "rf", style }) => (
  <div className={`tile ${on ? "on" : ""}`} style={style}>
    <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}>
      <div className="tile-bulb">
        <Icon d={I.bulb} size={18} stroke={1.7} style={{ color: on ? "#3a2400" : "var(--text-mute)" }}/>
      </div>
      <div className={`sw ${on ? "on" : ""}`}/>
    </div>
    <div style={{ display: "flex", flexDirection: "column", gap: 2, marginTop: 2 }}>
      <div style={{ fontWeight: 600, fontSize: 15 }}>{name}</div>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <div style={{ color: "var(--text-mute)", fontSize: 12 }}>
          {on ? (dim != null ? `On · ${dim}%` : "On") : "Off"} {room ? `· ${room}` : ""}
        </div>
        <ProtocolBadge kind={protocol}/>
      </div>
      {on && dim != null && (
        <div className="rail" style={{ marginTop: 6 }}><i style={{ width: `${dim}%` }}/></div>
      )}
    </div>
  </div>
);

// ── 1. HOME ──────────────────────────────────────────────────

function HomeScreen() {
  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "hidden", paddingBottom: 90 }}>
      <StatusBarPad/>

      {/* greeting */}
      <div style={{ padding: `8px ${PAD}px 4px`, display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}>
        <div>
          <div style={{ color: "var(--text-mute)", fontSize: 13, fontWeight: 500 }}>Tuesday, 9:41</div>
          <h1 style={{ fontSize: 30, fontWeight: 600, marginTop: 4, letterSpacing: "-0.03em" }}>
            Good evening,<br/>
            <span style={{ color: "var(--text-mute)" }}>Mira</span>
          </h1>
        </div>
        <div style={{ display: "flex", gap: 8 }}>
          <button className="chip" style={{ width: 36, height: 36, padding: 0, justifyContent: "center" }}>
            <Icon d={I.search} size={16} stroke={1.7}/>
          </button>
          <div style={{ position: "relative" }}>
            <button className="chip" style={{ width: 36, height: 36, padding: 0, justifyContent: "center" }}>
              <Icon d={I.bell} size={16} stroke={1.7}/>
            </button>
            <span style={{ position: "absolute", top: 6, right: 6, width: 7, height: 7, borderRadius: "50%", background: "var(--on)" }}/>
          </div>
        </div>
      </div>

      {/* HERO — whole home */}
      <div style={{ padding: `18px ${PAD}px 0` }}>
        <div className="tile on" style={{ padding: 20, gap: 16 }}>
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 12 }}>
            <div style={{ minWidth: 0 }}>
              <div style={{ color: "var(--on)", fontSize: 11, fontFamily: "var(--font-mono)", letterSpacing: "0.1em", textTransform: "uppercase" }}>Whole home</div>
              <div style={{ marginTop: 8, display: "flex", alignItems: "baseline", gap: 10, whiteSpace: "nowrap" }}>
                <span className="num-display" style={{ fontSize: 56 }}>7</span>
                <span style={{ color: "var(--text-mute)", fontSize: 14 }}>of 23 on</span>
              </div>
            </div>
            <div className="sw-big on" style={{ flexShrink: 0 }}/>
          </div>
          <div style={{ display: "flex", alignItems: "center", color: "var(--text-mute)", fontSize: 12, gap: 8, whiteSpace: "nowrap" }}>
            <Icon d={I.energy} size={13} stroke={1.7} style={{ color: "var(--on)" }}/>
            <span><span className="mono" style={{ color: "var(--text)" }}>184 W</span> now</span>
            <span style={{ color: "var(--text-dim)" }}>·</span>
            <span><span className="mono" style={{ color: "var(--text)" }}>3.2 kWh</span> today</span>
            <span style={{ marginLeft: "auto", color: "var(--text-dim)" }}>21° inside</span>
          </div>
        </div>
      </div>

      {/* scenes row */}
      <SectionHead title="Scenes" right={<button style={{ color: "var(--text-mute)", fontSize: 13 }}>All</button>}/>
      <div className="h-scroll" style={{ marginBottom: 4 }}>
        {[
          { name: "Evening",    sub: "8 devices", c: "var(--on)",  active: true },
          { name: "Goodnight",  sub: "All off",   c: "var(--cool)" },
          { name: "Movie",      sub: "Living rm", c: "#a96bd9" },
          { name: "Read",       sub: "Bedroom",   c: "#d97a45" },
          { name: "Away",       sub: "Everything off", c: "var(--text-mute)" },
        ].map(s => (
          <button key={s.name} className="card" style={{ width: 130, height: 110, padding: 14, display: "flex", flexDirection: "column", justifyContent: "space-between", alignItems: "flex-start", textAlign: "left", borderColor: s.active ? "var(--on)" : "var(--hairline)" }}>
            <div style={{ width: 26, height: 26, borderRadius: "50%", background: s.c, opacity: s.active ? 1 : 0.4 }}/>
            <div>
              <div style={{ fontWeight: 600, fontSize: 14 }}>{s.name}</div>
              <div style={{ color: "var(--text-mute)", fontSize: 11.5, marginTop: 2 }}>{s.sub}</div>
            </div>
          </button>
        ))}
      </div>

      {/* favorites grid */}
      <SectionHead title="Favorites" right={
        <div style={{ display: "flex", gap: 4 }}>
          <button className="chip active" style={{ padding: "5px 10px", fontSize: 12 }}>All</button>
          <button className="chip" style={{ padding: "5px 10px", fontSize: 12 }}>On</button>
          <button className="chip" style={{ padding: "5px 10px", fontSize: 12 }}>Lights</button>
        </div>
      }/>
      <div style={{ padding: `0 ${PAD}px`, display: "grid", gridTemplateColumns: "1fr 1fr", gap: 10 }}>
        <DeviceTile name="Floor lamp"   room="Living"  on dim={62} protocol="matter"/>
        <DeviceTile name="TV strip"     room="Living"  on dim={28} protocol="matter"/>
        <DeviceTile name="Kitchen isle" room="Kitchen" on dim={100} protocol="wifi"/>
        <DeviceTile name="Coffee bar"   room="Kitchen"          protocol="rf"/>
        <DeviceTile name="Porch"        room="Outside" on        protocol="rf"/>
        <DeviceTile name="Nightstand"   room="Bedroom"           protocol="matter"/>
      </div>

      <TabBar active="home"/>
    </div>
  );
}

// ── 2. ROOMS ──────────────────────────────────────────────────

function RoomsScreen() {
  const rooms = [
    { name: "Living room", on: 3, total: 5, temp: "21°", img: "warm" },
    { name: "Kitchen",     on: 2, total: 4, temp: "22°", img: "warm" },
    { name: "Bedroom",     on: 0, total: 4, temp: "19°", img: "cool" },
    { name: "Bathroom",    on: 0, total: 2, temp: "—",   img: "cool" },
    { name: "Hallway",     on: 1, total: 3, temp: "—",   img: "neutral" },
    { name: "Outside",     on: 1, total: 5, temp: "12°", img: "cool" },
  ];
  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "hidden", paddingBottom: 90 }}>
      <StatusBarPad/>
      <div style={{ padding: `8px ${PAD}px 0`, display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <h1 style={{ fontSize: 30, fontWeight: 600, letterSpacing: "-0.03em" }}>Rooms</h1>
        <button className="chip" style={{ width: 36, height: 36, padding: 0, justifyContent: "center" }}>
          <Icon d={I.plus} size={16} stroke={2}/>
        </button>
      </div>
      <div style={{ padding: `0 ${PAD}px`, color: "var(--text-mute)", fontSize: 13, marginTop: 2 }}>6 rooms · 7 lights on</div>

      <div style={{ padding: `24px ${PAD}px 0`, display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12 }}>
        {rooms.map(r => {
          const active = r.on > 0;
          const grad =
            r.img === "warm" ? "linear-gradient(155deg, #3a2f1f 0%, #271f14 100%)" :
            r.img === "cool" ? "linear-gradient(155deg, #1f2a30 0%, #161c20 100%)" :
                               "linear-gradient(155deg, #2a2620 0%, #1d1a15 100%)";
          return (
            <div key={r.name} className="card" style={{ padding: 14, height: 150, background: active ? grad : "var(--card)", borderColor: active ? "transparent" : "var(--hairline)", display: "flex", flexDirection: "column", justifyContent: "space-between" }}>
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}>
                <span className="dot on" style={{ visibility: active ? "visible" : "hidden" }}/>
                <span style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-mute)" }}>{r.temp}</span>
              </div>
              <div>
                <div style={{ fontWeight: 600, fontSize: 16, marginBottom: 2 }}>{r.name}</div>
                <div style={{ color: "var(--text-mute)", fontSize: 12.5 }}>
                  <span className="mono" style={{ color: active ? "var(--on)" : "var(--text-mute)" }}>{r.on}</span>
                  <span style={{ color: "var(--text-dim)" }}> / {r.total}</span> on
                </div>
              </div>
            </div>
          );
        })}
      </div>

      <TabBar active="rooms"/>
    </div>
  );
}

// ── 3. LIGHT DETAIL ───────────────────────────────────────────

function LightDetailScreen() {
  const [bright, setBright] = React.useState(62);
  const [mode, setMode] = React.useState("color"); // color | white
  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "hidden", background: "linear-gradient(180deg, #2a2218 0%, var(--bg) 50%)" }}>
      <StatusBarPad/>
      {/* nav */}
      <div style={{ padding: `4px ${PAD}px`, display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <button className="chip" style={{ width: 36, height: 36, padding: 0, justifyContent: "center" }}>
          <Icon d={I.back} size={16} stroke={2}/>
        </button>
        <div style={{ textAlign: "center" }}>
          <div style={{ fontSize: 15, fontWeight: 600 }}>Floor lamp</div>
          <div style={{ color: "var(--text-mute)", fontSize: 11.5 }}>Living room · Matter</div>
        </div>
        <button className="chip" style={{ width: 36, height: 36, padding: 0, justifyContent: "center" }}>
          <Icon d={I.more} size={16} stroke={2}/>
        </button>
      </div>

      {/* color wheel + brightness */}
      <div style={{ padding: "28px 0 0", display: "flex", justifyContent: "center" }}>
        <div className="color-ring"/>
      </div>

      <div style={{ padding: `24px ${PAD}px 0`, display: "flex", flexDirection: "column", gap: 12 }}>
        <div className="rail-fat">
          <div className="fill" style={{ width: `${bright}%` }}/>
          <span className="label">
            <Icon d={I.sun} size={18} stroke={1.7} style={{ verticalAlign: "-3px", marginRight: 8 }}/>
            Brightness
          </span>
          <span className="pct">{bright}%</span>
        </div>

        {/* white/color segmented */}
        <div style={{ display: "flex", background: "var(--card)", borderRadius: 14, padding: 4, gap: 4 }}>
          {["color", "white"].map(m => (
            <button key={m} onClick={() => setMode(m)}
              style={{
                flex: 1, padding: "10px 0", borderRadius: 10,
                background: mode === m ? "var(--card-3)" : "transparent",
                color: mode === m ? "var(--text)" : "var(--text-mute)",
                fontWeight: 500, fontSize: 13.5
              }}>
              {m === "color" ? "Color" : "White"}
            </button>
          ))}
        </div>

        {/* quick presets */}
        <div style={{ marginTop: 6 }}>
          <div style={{ color: "var(--text-mute)", fontSize: 11.5, fontFamily: "var(--font-mono)", letterSpacing: "0.08em", textTransform: "uppercase", marginBottom: 8 }}>Presets</div>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(5, 1fr)", gap: 8 }}>
            {[
              { c: "#f5bd6e", n: "Warm" },
              { c: "#ffe9c4", n: "Soft" },
              { c: "#ffffff", n: "Bright" },
              { c: "#c4a4e0", n: "Lilac" },
              { c: "#7aa4d9", n: "Cool" },
            ].map(p => (
              <div key={p.n} style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 6 }}>
                <div style={{ width: 44, height: 44, borderRadius: "50%", background: p.c, boxShadow: p.n === "Warm" ? "0 0 0 2px var(--on), 0 0 0 4px var(--bg)" : "inset 0 0 0 1px rgba(0,0,0,0.1)" }}/>
                <span style={{ fontSize: 10.5, color: "var(--text-mute)" }}>{p.n}</span>
              </div>
            ))}
          </div>
        </div>

        {/* footer actions */}
        <div className="card" style={{ marginTop: 14, padding: 0, overflow: "hidden" }}>
          {[
            { d: I.schedule, l: "Schedule", v: "Off at 23:00" },
            { d: I.energy,   l: "Energy",   v: "26 W" },
            { d: I.settings, l: "Configure", v: null },
          ].map((row, i, a) => (
            <React.Fragment key={row.l}>
              <div style={{ display: "flex", alignItems: "center", padding: "14px 16px", gap: 12 }}>
                <Icon d={row.d} size={17} stroke={1.7} style={{ color: "var(--text-mute)" }}/>
                <span style={{ fontSize: 14, fontWeight: 500 }}>{row.l}</span>
                <span style={{ marginLeft: "auto", color: "var(--text-mute)", fontSize: 13 }}>{row.v}</span>
                <Icon d={I.chevR} size={14} stroke={1.7} style={{ color: "var(--text-dim)" }}/>
              </div>
              {i < a.length - 1 && <div className="sep" style={{ marginLeft: 16 }}/>}
            </React.Fragment>
          ))}
        </div>
      </div>
    </div>
  );
}

// ── 4. SCENES ─────────────────────────────────────────────────

function ScenesScreen() {
  const scenes = [
    { name: "Evening",   devices: 8, sub: "Warm low everywhere",        active: true,  hue: "var(--on)" },
    { name: "Goodnight", devices: 23, sub: "Everything off · away on",   hue: "var(--cool)" },
    { name: "Movie",     devices: 4,  sub: "Lights down, TV strip on",   hue: "#a96bd9" },
    { name: "Read",      devices: 2,  sub: "Reading nook + nightstand",  hue: "#d97a45" },
    { name: "Wake up",   devices: 6,  sub: "Gradual sunrise · 07:00",    hue: "#ffd066" },
    { name: "Away",      devices: 23, sub: "All off, porch on",          hue: "var(--text-mute)" },
  ];
  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "hidden", paddingBottom: 90 }}>
      <StatusBarPad/>
      <div style={{ padding: `8px ${PAD}px 0`, display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <h1 style={{ fontSize: 30, fontWeight: 600 }}>Scenes</h1>
        <button className="chip" style={{ padding: "8px 14px", fontSize: 13 }}>
          <Icon d={I.plus} size={14} stroke={2}/> New scene
        </button>
      </div>

      {/* big active scene */}
      <div style={{ padding: `18px ${PAD}px 0` }}>
        <div className="tile on" style={{ padding: 22, gap: 14 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
            <div style={{ width: 44, height: 44, borderRadius: 14, background: "var(--on)", display: "grid", placeItems: "center" }}>
              <Icon d={I.scenes} size={20} stroke={2} style={{ color: "#3a2400" }}/>
            </div>
            <div>
              <div style={{ color: "var(--on)", fontSize: 11, fontFamily: "var(--font-mono)", letterSpacing: "0.1em", textTransform: "uppercase" }}>Active</div>
              <div style={{ fontSize: 22, fontWeight: 600, marginTop: 2 }}>Evening</div>
            </div>
            <div style={{ marginLeft: "auto" }} className="sw-big on"/>
          </div>
          <div style={{ display: "flex", gap: 14, color: "var(--text-mute)", fontSize: 12.5 }}>
            <span><span className="mono" style={{ color: "var(--text)" }}>8</span> devices</span>
            <span style={{ color: "var(--text-dim)" }}>·</span>
            <span>Activated 18:42</span>
          </div>
        </div>
      </div>

      <SectionHead title="All scenes"/>
      <div style={{ padding: `0 ${PAD}px`, display: "grid", gridTemplateColumns: "1fr 1fr", gap: 10 }}>
        {scenes.slice(1).map(s => (
          <div key={s.name} className="card" style={{ padding: 14, height: 130, display: "flex", flexDirection: "column", justifyContent: "space-between" }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}>
              <div style={{ width: 32, height: 32, borderRadius: 10, background: "var(--card-3)", display: "grid", placeItems: "center" }}>
                <div style={{ width: 14, height: 14, borderRadius: "50%", background: s.hue }}/>
              </div>
              <button style={{ color: "var(--text-mute)", fontSize: 11 }}>Run</button>
            </div>
            <div>
              <div style={{ fontWeight: 600, fontSize: 15 }}>{s.name}</div>
              <div style={{ color: "var(--text-mute)", fontSize: 12, marginTop: 3, lineHeight: 1.3 }}>{s.sub}</div>
              <div style={{ color: "var(--text-dim)", fontSize: 11.5, marginTop: 6, fontFamily: "var(--font-mono)" }}>
                {s.devices} {s.devices === 1 ? "device" : "devices"}
              </div>
            </div>
          </div>
        ))}
      </div>

      <TabBar active="scenes"/>
    </div>
  );
}

// ── 5. SCHEDULES ──────────────────────────────────────────────

function SchedulesScreen() {
  // 24h timeline marks
  const events = [
    { at: 6.5,  d: "Coffee bar",        a: "on",  c: "var(--on)" },
    { at: 7,    d: "Wake up scene",     a: "run", c: "#ffd066" },
    { at: 8,    d: "Coffee bar",        a: "off", c: "var(--text-mute)" },
    { at: 17.5, d: "Porch",             a: "sunset", c: "#d97a45" },
    { at: 19,   d: "Evening scene",     a: "run", c: "var(--on)" },
    { at: 23,   d: "Hallway",           a: "off", c: "var(--text-mute)" },
  ];

  const schedules = [
    { name: "Porch lights",   sub: "At sunset",          days: "Every day",  time: "≈ 17:42", on: true,  ico: I.sun },
    { name: "Coffee bar",     sub: "Turn on",            days: "Mon–Fri",    time: "06:30",  on: true,  ico: I.energy },
    { name: "Hallway",        sub: "Turn off",           days: "Every day",  time: "23:00",  on: true,  ico: I.moon },
    { name: "Wake up scene",  sub: "Run gradually",      days: "Mon–Fri",    time: "07:00",  on: true,  ico: I.scenes },
    { name: "Away · all off", sub: "When everyone gone", days: "Conditional", time: "—",     on: false, ico: I.power },
  ];

  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "hidden", paddingBottom: 90 }}>
      <StatusBarPad/>
      <div style={{ padding: `8px ${PAD}px 0`, display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <h1 style={{ fontSize: 30, fontWeight: 600 }}>Schedules</h1>
        <button className="chip" style={{ padding: "8px 14px", fontSize: 13 }}>
          <Icon d={I.plus} size={14} stroke={2}/> New
        </button>
      </div>

      {/* timeline */}
      <div style={{ padding: `18px ${PAD}px 0` }}>
        <div className="card" style={{ padding: 18 }}>
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline", marginBottom: 14 }}>
            <div>
              <div style={{ color: "var(--text-mute)", fontSize: 11.5, fontFamily: "var(--font-mono)", letterSpacing: "0.1em", textTransform: "uppercase" }}>Today</div>
              <div style={{ fontSize: 16, fontWeight: 600, marginTop: 2 }}>5 events ahead</div>
            </div>
            <div style={{ color: "var(--text-mute)", fontSize: 12 }}>
              Next: <span className="mono" style={{ color: "var(--on)" }}>17:42</span>
            </div>
          </div>

          {/* 24h rail */}
          <div style={{ position: "relative", height: 60 }}>
            {/* day/night gradient */}
            <div style={{ position: "absolute", inset: 0, borderRadius: 12,
              background: "linear-gradient(90deg, #1a1d28 0%, #1a1d28 22%, #2a2618 28%, #3a2e1e 50%, #2a2618 72%, #1a1d28 78%, #1a1d28 100%)"
            }}/>
            {/* now indicator */}
            <div style={{ position: "absolute", top: -6, bottom: -6, left: "40%", width: 2, background: "var(--text)", borderRadius: 1 }}>
              <div style={{ position: "absolute", top: -10, left: -3, width: 8, height: 8, borderRadius: "50%", background: "var(--text)" }}/>
            </div>
            {/* events */}
            {events.map((e, i) => (
              <div key={i} style={{ position: "absolute", left: `${(e.at/24)*100}%`, top: 20, transform: "translateX(-50%)" }}>
                <div style={{ width: 10, height: 20, borderRadius: 3, background: e.c }}/>
              </div>
            ))}
          </div>
          {/* hour labels */}
          <div style={{ display: "flex", justifyContent: "space-between", marginTop: 8, color: "var(--text-dim)", fontSize: 10, fontFamily: "var(--font-mono)" }}>
            <span>00</span><span>06</span><span>12</span><span>18</span><span>24</span>
          </div>
        </div>
      </div>

      <SectionHead title="Automations"/>
      <div style={{ padding: `0 ${PAD}px`, display: "flex", flexDirection: "column", gap: 8 }}>
        {schedules.map(s => (
          <div key={s.name} className="card" style={{ padding: 14, flexDirection: "row", display: "flex", alignItems: "center", gap: 14, opacity: s.on ? 1 : 0.55 }}>
            <div style={{ width: 40, height: 40, borderRadius: 12, background: s.on ? "var(--on-soft)" : "var(--card-3)", display: "grid", placeItems: "center", flexShrink: 0 }}>
              <Icon d={s.ico} size={18} stroke={1.7} style={{ color: s.on ? "var(--on)" : "var(--text-mute)" }}/>
            </div>
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline" }}>
                <div style={{ fontWeight: 600, fontSize: 14.5 }}>{s.name}</div>
                <div className="mono" style={{ fontSize: 13, color: s.on ? "var(--on)" : "var(--text-mute)" }}>{s.time}</div>
              </div>
              <div style={{ color: "var(--text-mute)", fontSize: 12, marginTop: 2, display: "flex", gap: 6 }}>
                <span>{s.sub}</span>
                <span style={{ color: "var(--text-dim)" }}>·</span>
                <span>{s.days}</span>
              </div>
            </div>
            <div className={`sw ${s.on ? "on" : ""}`}/>
          </div>
        ))}
      </div>

      <TabBar active="schedule"/>
    </div>
  );
}

Object.assign(window, { HomeScreen, RoomsScreen, LightDetailScreen, ScenesScreen, SchedulesScreen, ProtocolBadge, DeviceTile, TabBar, SectionHead });
