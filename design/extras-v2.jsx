/* HomeHub — round 2 additions:
   · Insights / Energy (mobile + desktop)
   · Automations list + builder + desktop
   · Activity / History timeline (mobile + desktop)
   · iOS Lock Screen with HomeHub widgets
   · Apple Watch screens (Home / Scenes / Light)

   Same warm-dark / amber design language as the rest of the system. */

const PAD2 = 22;

// ─────────────────────────────────────────────────────────────
// shared mini-bits
// ─────────────────────────────────────────────────────────────

const StatusBarPad2 = () => (<div style={{ height: 54 }} />);

// little bar chart — driven by an array of 0..1 values
const SparkBars = ({ data, height = 96, gap = 3, peakIdx = -1, color = "var(--on)", dim = "var(--card-3)", showAxis = true }) => {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
      <div style={{ display: "flex", alignItems: "flex-end", gap, height }}>
        {data.map((v, i) => (
          <div key={i} style={{
            flex: 1,
            height: `${Math.max(2, v * 100)}%`,
            background: i === peakIdx ? color : dim,
            borderRadius: 2,
            boxShadow: i === peakIdx ? `0 0 12px ${color === "var(--on)" ? "rgba(245,189,110,0.5)" : "transparent"}` : "none",
          }}/>
        ))}
      </div>
      {showAxis && (
        <div style={{ display: "flex", justifyContent: "space-between", fontFamily: "var(--font-mono)", fontSize: 9.5, color: "var(--text-dim)" }}>
          <span>00</span><span>06</span><span>12</span><span>18</span><span>24</span>
        </div>
      )}
    </div>
  );
};

// donut chart — segments with hue + value (sum to 1)
const Donut = ({ segments, size = 140, stroke = 18, center }) => {
  const r = (size - stroke) / 2;
  const C = 2 * Math.PI * r;
  let offset = 0;
  return (
    <div style={{ position: "relative", width: size, height: size }}>
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} style={{ transform: "rotate(-90deg)" }}>
        <circle cx={size/2} cy={size/2} r={r} fill="none" stroke="var(--card-3)" strokeWidth={stroke}/>
        {segments.map((s, i) => {
          const len = s.v * C;
          const dash = `${len} ${C - len}`;
          const dashOffset = -offset;
          offset += len;
          return (
            <circle key={i} cx={size/2} cy={size/2} r={r} fill="none"
              stroke={s.c} strokeWidth={stroke} strokeDasharray={dash} strokeDashoffset={dashOffset}
              strokeLinecap="butt"/>
          );
        })}
      </svg>
      {center && (
        <div style={{ position: "absolute", inset: 0, display: "grid", placeItems: "center", textAlign: "center" }}>
          {center}
        </div>
      )}
    </div>
  );
};

const TabBar2 = ({ active }) => {
  const items = [
    { id: "home",      label: "Home",     d: I.home },
    { id: "rooms",     label: "Rooms",    d: I.rooms },
    { id: "scenes",    label: "Scenes",   d: I.scenes },
    { id: "schedule",  label: "Schedule", d: I.schedule },
    { id: "settings",  label: "Settings", d: I.settings },
  ];
  const idx = Math.max(0, items.findIndex(it => it.id === active));
  const slot = `(100% - 20px) / 5`;
  return (
    <div className="tabbar">
      <div className="tabdock">
        <i className="tab-lens" style={{ left: `calc(${idx} * (${slot}) + 10px)`, width: `calc(${slot})` }}/>
        {items.map(it => (
          <button key={it.id} aria-label={it.label} title={it.label} className={it.id === active ? "active" : ""}>
            <Icon d={it.d} size={22} stroke={1.7}/>
          </button>
        ))}
      </div>
    </div>
  );
};

// ─────────────────────────────────────────────────────────────
// INSIGHTS — mobile
// ─────────────────────────────────────────────────────────────

function InsightsScreen() {
  // hourly draw for "today" — peak at 19:00 (evening scene)
  const hours = [0.04,0.04,0.03,0.03,0.03,0.05,0.10,0.18,0.22,0.15,0.10,0.12,
                 0.16,0.14,0.12,0.16,0.32,0.55,0.78,0.95,0.74,0.52,0.30,0.14];
  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "auto", paddingBottom: 100 }}>
      <StatusBarPad2/>

      <div style={{ padding: `8px ${PAD2}px 0`, display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <h1 style={{ fontSize: 30, fontWeight: 600, letterSpacing: "-0.03em" }}>Insights</h1>
        <button className="chip" style={{ width: 36, height: 36, padding: 0, justifyContent: "center" }}>
          <Icon d={I.more} size={16} stroke={2}/>
        </button>
      </div>

      <div style={{ padding: `4px ${PAD2}px 0`, color: "var(--text-mute)", fontSize: 13 }}>
        Energy use, room by room
      </div>

      {/* range picker */}
      <div style={{ padding: `16px ${PAD2}px 0`, display: "flex", gap: 6 }}>
        {["Today","Week","Month","Year"].map((r,i) => (
          <button key={r} className={`chip ${i===0 ? "active":""}`} style={{ padding: "6px 12px", fontSize: 12.5 }}>{r}</button>
        ))}
      </div>

      {/* hero card — kWh today + bar chart */}
      <div style={{ padding: `16px ${PAD2}px 0` }}>
        <div className="card" style={{ padding: 18 }}>
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-end", marginBottom: 16 }}>
            <div>
              <div style={{ color: "var(--text-mute)", fontSize: 11, fontFamily: "var(--font-mono)", letterSpacing: "0.1em", textTransform: "uppercase" }}>kWh today</div>
              <div style={{ display: "flex", alignItems: "baseline", gap: 8, marginTop: 6 }}>
                <span className="num-display" style={{ fontSize: 44 }}>3.2</span>
                <span style={{ color: "var(--good)", fontSize: 13, fontFamily: "var(--font-mono)" }}>−12%</span>
              </div>
              <div style={{ color: "var(--text-dim)", fontSize: 12, marginTop: 2 }}>vs. last Tuesday</div>
            </div>
            <div style={{ textAlign: "right" }}>
              <div className="mono" style={{ fontSize: 16, color: "var(--text)" }}>$0.48</div>
              <div style={{ color: "var(--text-dim)", fontSize: 11.5 }}>est. cost</div>
            </div>
          </div>
          <SparkBars data={hours} peakIdx={19} height={120}/>
          <div style={{ marginTop: 12, padding: 10, background: "var(--on-soft)", borderRadius: 10, fontSize: 12, color: "var(--on)", display: "flex", alignItems: "center", gap: 8 }}>
            <Icon d={I.energy} size={13} stroke={2}/>
            <span style={{ color: "var(--text)" }}>Peak at <span className="mono">19:00</span> — Evening scene ran for 2h 14m</span>
          </div>
        </div>
      </div>

      {/* by room donut + legend */}
      <div style={{ padding: `16px ${PAD2}px 0` }}>
        <div className="card" style={{ padding: 18 }}>
          <div style={{ color: "var(--text-mute)", fontSize: 11, fontFamily: "var(--font-mono)", letterSpacing: "0.1em", textTransform: "uppercase", marginBottom: 14 }}>By room</div>
          <div style={{ display: "flex", gap: 18, alignItems: "center" }}>
            <Donut
              size={130}
              stroke={20}
              segments={[
                { v: 0.42, c: "#f5bd6e" },
                { v: 0.28, c: "#84acc4" },
                { v: 0.16, c: "#c4a4e0" },
                { v: 0.09, c: "#9cc28a" },
                { v: 0.05, c: "#e08a7a" },
              ]}
              center={
                <div>
                  <div className="num-display" style={{ fontSize: 22 }}>23</div>
                  <div style={{ color: "var(--text-dim)", fontSize: 10, fontFamily: "var(--font-mono)" }}>kWh</div>
                </div>
              }
            />
            <div style={{ flex: 1, display: "flex", flexDirection: "column", gap: 10 }}>
              {[
                { c: "#f5bd6e", l: "Living room", v: "9.7 kWh" },
                { c: "#84acc4", l: "Kitchen",     v: "6.4 kWh" },
                { c: "#c4a4e0", l: "Bedroom",     v: "3.7 kWh" },
                { c: "#9cc28a", l: "Outside",     v: "2.1 kWh" },
                { c: "#e08a7a", l: "Hallway",     v: "1.1 kWh" },
              ].map(r => (
                <div key={r.l} style={{ display: "flex", alignItems: "center", gap: 10 }}>
                  <div style={{ width: 8, height: 8, borderRadius: 2, background: r.c }}/>
                  <div style={{ flex: 1, fontSize: 13 }}>{r.l}</div>
                  <div className="mono" style={{ fontSize: 12, color: "var(--text-mute)" }}>{r.v}</div>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>

      {/* top consumers list */}
      <div style={{ padding: `20px ${PAD2}px 0` }}>
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline", marginBottom: 10 }}>
          <h2 style={{ fontSize: 15, fontWeight: 600 }}>Top consumers</h2>
          <span style={{ color: "var(--text-dim)", fontSize: 12, fontFamily: "var(--font-mono)" }}>this week</span>
        </div>
        <div className="card" style={{ padding: 0 }}>
          {[
            { n: "Kitchen isle",  r: "Kitchen",     w: 0.92, kwh: "5.1 kWh" },
            { n: "Floor lamp",    r: "Living room", w: 0.74, kwh: "4.0 kWh" },
            { n: "TV strip",      r: "Living room", w: 0.48, kwh: "2.6 kWh" },
            { n: "Hallway",       r: "Hallway",     w: 0.31, kwh: "1.7 kWh" },
            { n: "Porch",         r: "Outside",     w: 0.22, kwh: "1.2 kWh" },
          ].map((d, i, arr) => (
            <div key={d.n} style={{ padding: "12px 14px", borderBottom: i < arr.length-1 ? "1px solid var(--hairline)" : "none" }}>
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 6 }}>
                <div>
                  <div style={{ fontSize: 13.5, fontWeight: 500 }}>{d.n}</div>
                  <div style={{ color: "var(--text-mute)", fontSize: 11.5 }}>{d.r}</div>
                </div>
                <div className="mono" style={{ fontSize: 12.5, color: "var(--text-mute)" }}>{d.kwh}</div>
              </div>
              <div className="rail"><i style={{ width: `${d.w*100}%` }}/></div>
            </div>
          ))}
        </div>
      </div>

      <TabBar2 active="home"/>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// AUTOMATIONS — mobile list
// ─────────────────────────────────────────────────────────────

function AutomationsScreen() {
  const autos = [
    { name: "Sunset porch",          when: "Sunset",            then: "Porch on · 80%",                on: true,  runs: 184 },
    { name: "Goodnight",             when: "23:00 weekdays",    then: "All off · Lock door",            on: true,  runs: 96  },
    { name: "Motion — hallway",      when: "Motion after dark", then: "Hallway 30% · 2 min",            on: true,  runs: 412 },
    { name: "Wake gradient",         when: "06:45 weekdays",    then: "Bedroom 10→100% over 8 min",     on: false, runs: 0   },
    { name: "Away — no one home",    when: "Everyone left",     then: "Everything off · Camera arm",    on: true,  runs: 12  },
    { name: "Coffee bar",            when: "06:30 weekdays",    then: "Coffee bar on · 20 min",         on: false, runs: 0   },
    { name: "Movie mode",            when: "TV turns on",       then: "Lights 15% · Strip 40%",         on: true,  runs: 27  },
  ];
  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "auto", paddingBottom: 100 }}>
      <StatusBarPad2/>

      <div style={{ padding: `8px ${PAD2}px 0`, display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <h1 style={{ fontSize: 30, fontWeight: 600, letterSpacing: "-0.03em" }}>Automations</h1>
        <button className="chip" style={{ width: 36, height: 36, padding: 0, justifyContent: "center", background: "var(--on)", color: "#1a1813", borderColor: "var(--on)" }}>
          <Icon d={I.plus} size={16} stroke={2.2}/>
        </button>
      </div>
      <div style={{ padding: `4px ${PAD2}px 0`, color: "var(--text-mute)", fontSize: 13 }}>
        5 enabled · 731 runs this month
      </div>

      {/* tag filter */}
      <div className="h-scroll" style={{ marginTop: 18 }}>
        {["All","Triggered","Time","Sensor","Manual"].map((t,i) => (
          <button key={t} className={`chip ${i===0?"active":""}`} style={{ padding: "7px 14px" }}>{t}</button>
        ))}
      </div>

      <div style={{ padding: `18px ${PAD2}px 0`, display: "flex", flexDirection: "column", gap: 10 }}>
        {autos.map(a => (
          <div key={a.name} className="card" style={{ padding: 14, opacity: a.on ? 1 : 0.62 }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 12 }}>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 8 }}>
                  <span className="dot" style={{ background: a.on ? "var(--on)" : "var(--text-dim)", boxShadow: a.on ? "0 0 0 4px var(--on-soft)" : "none" }}/>
                  <div style={{ fontWeight: 600, fontSize: 14.5 }}>{a.name}</div>
                </div>
                {/* WHEN → THEN */}
                <div style={{ display: "flex", alignItems: "center", gap: 8, fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-mute)", flexWrap: "wrap" }}>
                  <span style={{ color: "var(--cool)", textTransform: "uppercase", letterSpacing: "0.06em" }}>WHEN</span>
                  <span style={{ color: "var(--text)" }}>{a.when}</span>
                  <Icon d={I.chevR} size={12} stroke={2} style={{ color: "var(--text-dim)" }}/>
                  <span style={{ color: "var(--on)", textTransform: "uppercase", letterSpacing: "0.06em" }}>THEN</span>
                  <span style={{ color: "var(--text)" }}>{a.then}</span>
                </div>
                {a.runs > 0 && (
                  <div style={{ marginTop: 8, fontSize: 11.5, color: "var(--text-dim)", fontFamily: "var(--font-mono)" }}>
                    ran {a.runs}× this month · last {a.runs > 100 ? "2h ago" : "yesterday"}
                  </div>
                )}
              </div>
              <div className={`sw ${a.on ? "on" : ""}`} style={{ marginTop: 4 }}/>
            </div>
          </div>
        ))}
      </div>

      <TabBar2 active="schedule"/>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// AUTOMATION BUILDER — mobile (step UI)
// ─────────────────────────────────────────────────────────────

function AutomationBuilderScreen() {
  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "auto", paddingBottom: 100 }}>
      <StatusBarPad2/>

      {/* header */}
      <div style={{ padding: `4px ${PAD2}px`, display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <button className="chip" style={{ width: 36, height: 36, padding: 0, justifyContent: "center" }}>
          <Icon d={I.close} size={16} stroke={2}/>
        </button>
        <div style={{ textAlign: "center" }}>
          <div style={{ fontSize: 15, fontWeight: 600 }}>New automation</div>
          <div style={{ color: "var(--text-mute)", fontSize: 11.5 }}>Step 3 of 4</div>
        </div>
        <button style={{ fontSize: 13.5, fontWeight: 500, color: "var(--on)" }}>Save</button>
      </div>

      {/* name */}
      <div style={{ padding: `24px ${PAD2}px 0` }}>
        <div style={{ color: "var(--text-mute)", fontSize: 11.5, fontFamily: "var(--font-mono)", letterSpacing: "0.1em", textTransform: "uppercase", marginBottom: 8 }}>Name</div>
        <div className="card" style={{ padding: "14px 16px" }}>
          <span style={{ fontSize: 16, fontWeight: 500 }}>Wake gradient</span>
        </div>
      </div>

      {/* WHEN block */}
      <div style={{ padding: `22px ${PAD2}px 0` }}>
        <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 10 }}>
          <span style={{ color: "var(--cool)", fontFamily: "var(--font-mono)", fontSize: 11, letterSpacing: "0.1em", textTransform: "uppercase" }}>When</span>
          <div style={{ flex: 1, height: 1, background: "var(--hairline)" }}/>
        </div>
        <div className="card" style={{ padding: 0, borderLeft: "3px solid var(--cool)" }}>
          <div style={{ padding: "14px 16px", borderBottom: "1px solid var(--hairline)", display: "flex", justifyContent: "space-between", alignItems: "center" }}>
            <div>
              <div style={{ fontSize: 13.5, fontWeight: 500 }}>Time of day</div>
              <div className="mono" style={{ color: "var(--text-mute)", fontSize: 11.5, marginTop: 2 }}>06:45</div>
            </div>
            <Icon d={I.chevR} size={14} stroke={2} style={{ color: "var(--text-dim)" }}/>
          </div>
          <div style={{ padding: "14px 16px", display: "flex", justifyContent: "space-between", alignItems: "center" }}>
            <div>
              <div style={{ fontSize: 13.5, fontWeight: 500 }}>On days</div>
              <div style={{ color: "var(--text-mute)", fontSize: 11.5, marginTop: 2 }}>Mon Tue Wed Thu Fri</div>
            </div>
            <Icon d={I.chevR} size={14} stroke={2} style={{ color: "var(--text-dim)" }}/>
          </div>
        </div>
      </div>

      {/* IF block */}
      <div style={{ padding: `22px ${PAD2}px 0` }}>
        <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 10 }}>
          <span style={{ color: "var(--text-mute)", fontFamily: "var(--font-mono)", fontSize: 11, letterSpacing: "0.1em", textTransform: "uppercase" }}>And if</span>
          <div style={{ flex: 1, height: 1, background: "var(--hairline)" }}/>
        </div>
        <div className="card" style={{ padding: "14px 16px", display: "flex", alignItems: "center", justifyContent: "space-between" }}>
          <div style={{ color: "var(--text-mute)", fontSize: 13.5, display: "flex", alignItems: "center", gap: 8 }}>
            <Icon d={I.plus} size={14} stroke={2} style={{ color: "var(--on)" }}/>
            Add a condition (optional)
          </div>
          <Icon d={I.chevR} size={14} stroke={2} style={{ color: "var(--text-dim)" }}/>
        </div>
      </div>

      {/* THEN block */}
      <div style={{ padding: `22px ${PAD2}px 0` }}>
        <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 10 }}>
          <span style={{ color: "var(--on)", fontFamily: "var(--font-mono)", fontSize: 11, letterSpacing: "0.1em", textTransform: "uppercase" }}>Then</span>
          <div style={{ flex: 1, height: 1, background: "var(--hairline)" }}/>
        </div>
        <div className="card" style={{ padding: 0, borderLeft: "3px solid var(--on)" }}>
          {[
            { l: "Bedroom main", v: "Fade 10% → 100% over 8 min", d: I.bulb },
            { l: "Coffee bar",   v: "Turn on", d: I.bulb },
            { l: "Open blinds",  v: "After 4 min", d: I.sliders },
          ].map((s, i, a) => (
            <div key={s.l} style={{ padding: "14px 16px", borderBottom: i < a.length-1 ? "1px solid var(--hairline)" : "none", display: "flex", alignItems: "center", gap: 12 }}>
              <div style={{ width: 32, height: 32, borderRadius: 8, background: "var(--card-3)", display: "grid", placeItems: "center", color: "var(--on)" }}>
                <Icon d={s.d} size={15} stroke={1.7}/>
              </div>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontSize: 13.5, fontWeight: 500 }}>{s.l}</div>
                <div style={{ color: "var(--text-mute)", fontSize: 11.5, marginTop: 2 }}>{s.v}</div>
              </div>
              <Icon d={I.chevR} size={14} stroke={2} style={{ color: "var(--text-dim)" }}/>
            </div>
          ))}
        </div>
        <button className="card" style={{ width: "100%", marginTop: 8, padding: "12px 16px", textAlign: "left", display: "flex", alignItems: "center", gap: 8, color: "var(--text-mute)", fontSize: 13.5, background: "transparent", border: "1px dashed var(--border)" }}>
          <Icon d={I.plus} size={14} stroke={2} style={{ color: "var(--on)" }}/>
          Add an action
        </button>
      </div>

      {/* bottom CTA */}
      <div style={{ position: "absolute", bottom: 0, left: 0, right: 0, padding: `12px ${PAD2}px 36px`, background: "linear-gradient(to top, var(--bg) 70%, transparent)" }}>
        <button style={{
          width: "100%",
          padding: "16px",
          borderRadius: 14,
          background: "var(--on)",
          color: "#1a1813",
          fontWeight: 600,
          fontSize: 15,
        }}>Continue</button>
      </div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// ACTIVITY — mobile
// ─────────────────────────────────────────────────────────────

function ActivityScreen() {
  const groups = [
    {
      label: "Today",
      events: [
        { t: "19:42", who: "Auto",  what: "Sunset porch",       det: "Porch on · 80%",                  k: "auto" },
        { t: "19:01", who: "Mira",  what: "Evening scene",       det: "8 devices changed",                k: "scene" },
        { t: "18:14", who: "Auto",  what: "Motion — hallway",   det: "Hallway 30% for 2 min",            k: "auto" },
        { t: "12:03", who: "Mira",  what: "Kitchen isle",       det: "Brightness 80 → 100%",             k: "device" },
        { t: "06:31", who: "Auto",  what: "Coffee bar",         det: "Coffee bar on (disabled later)",   k: "auto" },
      ],
    },
    {
      label: "Yesterday",
      events: [
        { t: "23:01", who: "Auto",  what: "Goodnight",          det: "All off · Door locked",            k: "auto" },
        { t: "22:18", who: "Dad",   what: "Bedroom main",       det: "Turned off via Watch",             k: "device" },
        { t: "18:42", who: "Mira",  what: "Evening scene",       det: "8 devices changed",                k: "scene" },
        { t: "14:30", who: "Hub",   what: "Floor lamp",         det: "Reconnected · was offline 4 min",  k: "system" },
      ],
    },
  ];
  const kindStyle = {
    auto:   { c: "var(--on)",   d: I.scenes },
    scene:  { c: "#c4a4e0",     d: I.star },
    device: { c: "var(--cool)", d: I.bulb },
    system: { c: "var(--text-dim)", d: I.settings },
  };
  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "auto", paddingBottom: 100 }}>
      <StatusBarPad2/>

      <div style={{ padding: `8px ${PAD2}px 0`, display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <h1 style={{ fontSize: 30, fontWeight: 600, letterSpacing: "-0.03em" }}>Activity</h1>
        <button className="chip" style={{ width: 36, height: 36, padding: 0, justifyContent: "center" }}>
          <Icon d={I.search} size={16} stroke={1.7}/>
        </button>
      </div>
      <div style={{ padding: `4px ${PAD2}px 0`, color: "var(--text-mute)", fontSize: 13 }}>
        Everything that happened in your home
      </div>

      <div className="h-scroll" style={{ marginTop: 18 }}>
        {["All","Automations","People","Devices","System"].map((t,i) => (
          <button key={t} className={`chip ${i===0?"active":""}`} style={{ padding: "7px 14px" }}>{t}</button>
        ))}
      </div>

      <div style={{ padding: `18px ${PAD2}px 0` }}>
        {groups.map(g => (
          <div key={g.label} style={{ marginBottom: 18 }}>
            <div style={{ color: "var(--text-dim)", fontSize: 11.5, fontFamily: "var(--font-mono)", letterSpacing: "0.1em", textTransform: "uppercase", marginBottom: 10 }}>{g.label}</div>
            <div style={{ position: "relative" }}>
              {/* timeline line */}
              <div style={{ position: "absolute", left: 18, top: 14, bottom: 14, width: 1, background: "var(--hairline)" }}/>
              {g.events.map((e, i) => {
                const k = kindStyle[e.k];
                return (
                  <div key={i} style={{ display: "flex", gap: 14, padding: "10px 0", alignItems: "flex-start", position: "relative" }}>
                    <div style={{ width: 36, height: 36, borderRadius: "50%", background: "var(--card)", border: "1px solid var(--hairline)", display: "grid", placeItems: "center", color: k.c, flexShrink: 0, zIndex: 1 }}>
                      <Icon d={k.d} size={14} stroke={1.8}/>
                    </div>
                    <div style={{ flex: 1, minWidth: 0, paddingTop: 2 }}>
                      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline", gap: 8 }}>
                        <div style={{ fontSize: 13.5, fontWeight: 500 }}>{e.what}</div>
                        <div className="mono" style={{ fontSize: 11, color: "var(--text-dim)", flexShrink: 0 }}>{e.t}</div>
                      </div>
                      <div style={{ color: "var(--text-mute)", fontSize: 12, marginTop: 2 }}>
                        <span style={{ color: k.c }}>{e.who}</span> · {e.det}
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        ))}
      </div>

      <TabBar2 active="settings"/>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// iOS LOCK SCREEN with HomeHub widgets
// ─────────────────────────────────────────────────────────────

function IOSLockScreen() {
  return (
    <div className="hh" style={{
      position: "relative", height: "100%", overflow: "hidden",
      background: "radial-gradient(ellipse at 30% 20%, #322a1c 0%, #16140f 60%, #0a0a08 100%)",
      color: "#fff",
    }}>
      <StatusBarPad2/>

      {/* lock icon */}
      <div style={{ display: "flex", justifyContent: "center", marginTop: 10 }}>
        <div style={{ width: 26, height: 26, borderRadius: 6, background: "rgba(255,255,255,0.1)", display: "grid", placeItems: "center" }}>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#fff" strokeWidth="2"><path d="M5 11h14v10H5zM8 11V7a4 4 0 0 1 8 0v4"/></svg>
        </div>
      </div>

      {/* date + time */}
      <div style={{ textAlign: "center", marginTop: 18, padding: `0 ${PAD2}px` }}>
        <div style={{ fontSize: 14, fontWeight: 500, opacity: 0.85 }}>Tuesday, May 24</div>
        <div style={{ fontSize: 90, fontWeight: 200, letterSpacing: "-0.04em", marginTop: 4, lineHeight: 1, fontFamily: "ui-rounded, -apple-system, system-ui, sans-serif" }}>9:41</div>
      </div>

      {/* tiny weather-line widget */}
      <div style={{ display: "flex", justifyContent: "space-around", padding: `18px ${PAD2}px 0`, fontSize: 12, opacity: 0.85 }}>
        <span style={{ display: "flex", alignItems: "center", gap: 4 }}>
          <Icon d={I.sun} size={13} stroke={1.7}/>
          17° · Sunset 19:42
        </span>
      </div>

      {/* HomeHub widgets — 2 medium + 1 small row */}
      <div style={{ padding: `24px ${PAD2}px 0`, display: "flex", flexDirection: "column", gap: 12 }}>
        {/* medium widget — favorites */}
        <div style={{ background: "rgba(28,26,21,0.7)", backdropFilter: "blur(20px)", border: "1px solid rgba(255,255,255,0.08)", borderRadius: 22, padding: 14 }}>
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 12 }}>
            <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
              <div style={{ width: 14, height: 14, borderRadius: 4, background: "var(--on)" }}/>
              <span style={{ fontSize: 11, fontWeight: 600, letterSpacing: "0.04em", textTransform: "uppercase" }}>HomeHub</span>
            </div>
            <span className="mono" style={{ fontSize: 10, opacity: 0.5 }}>7 ON · 184 W</span>
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 8 }}>
            {[
              { n: "Living", on: true,  c: "var(--on)" },
              { n: "Strip",  on: true,  c: "var(--on)" },
              { n: "Porch",  on: true,  c: "var(--on)" },
              { n: "Coffee", on: false, c: "rgba(255,255,255,0.2)" },
            ].map(d => (
              <div key={d.n} style={{ background: d.on ? "rgba(245,189,110,0.16)" : "rgba(255,255,255,0.05)", borderRadius: 14, padding: 10, height: 64, display: "flex", flexDirection: "column", justifyContent: "space-between" }}>
                <div style={{ width: 18, height: 18, borderRadius: "50%", background: d.c, boxShadow: d.on ? "0 0 10px var(--on-glow)" : "none" }}/>
                <div style={{ fontSize: 9.5, fontWeight: 500, opacity: d.on ? 1 : 0.6 }}>{d.n}</div>
              </div>
            ))}
          </div>
        </div>

        {/* small row: 2 small widgets */}
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12 }}>
          {/* small — evening scene */}
          <div style={{ background: "linear-gradient(155deg, rgba(245,189,110,0.22), rgba(30,25,17,0.7))", backdropFilter: "blur(20px)", border: "1px solid rgba(245,189,110,0.18)", borderRadius: 22, padding: 14, aspectRatio: "1 / 1", display: "flex", flexDirection: "column", justifyContent: "space-between" }}>
            <div style={{ width: 28, height: 28, borderRadius: "50%", background: "var(--on)", boxShadow: "0 0 20px var(--on-glow)" }}/>
            <div>
              <div style={{ fontSize: 9.5, opacity: 0.7, fontFamily: "var(--font-mono)", letterSpacing: "0.08em", textTransform: "uppercase" }}>Active scene</div>
              <div style={{ fontSize: 16, fontWeight: 600, marginTop: 2 }}>Evening</div>
              <div style={{ fontSize: 10, opacity: 0.6, marginTop: 2 }}>since 18:42</div>
            </div>
          </div>
          {/* small — energy */}
          <div style={{ background: "rgba(28,26,21,0.7)", backdropFilter: "blur(20px)", border: "1px solid rgba(255,255,255,0.08)", borderRadius: 22, padding: 14, aspectRatio: "1 / 1", display: "flex", flexDirection: "column", justifyContent: "space-between" }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}>
              <span style={{ fontSize: 9.5, opacity: 0.6, fontFamily: "var(--font-mono)", letterSpacing: "0.08em", textTransform: "uppercase" }}>Today</span>
              <Icon d={I.energy} size={12} stroke={2} style={{ color: "var(--on)" }}/>
            </div>
            <div style={{ display: "flex", alignItems: "flex-end", gap: 2, height: 28 }}>
              {[0.2,0.3,0.4,0.3,0.5,0.6,0.8,0.95,0.7,0.5].map((v,i) => (
                <div key={i} style={{ flex: 1, height: `${v*100}%`, background: i===7?"var(--on)":"rgba(255,255,255,0.18)", borderRadius: 1 }}/>
              ))}
            </div>
            <div>
              <div className="num-display" style={{ fontSize: 22 }}>3.2<span style={{ fontSize: 11, opacity: 0.5, marginLeft: 3 }}>kWh</span></div>
              <div style={{ fontSize: 10, color: "#9cc28a", marginTop: 2 }}>−12% vs. last Tue</div>
            </div>
          </div>
        </div>
      </div>

      {/* flashlight + camera shortcuts */}
      <div style={{ position: "absolute", bottom: 36, left: 0, right: 0, display: "flex", justifyContent: "space-between", padding: `0 ${PAD2 + 10}px` }}>
        <div style={{ width: 44, height: 44, borderRadius: "50%", background: "rgba(255,255,255,0.12)", display: "grid", placeItems: "center" }}>
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#fff" strokeWidth="1.6"><path d="M9 3h6l-1 6h-4zM10 9v12l2-2 2 2V9"/></svg>
        </div>
        <div style={{ width: 44, height: 44, borderRadius: "50%", background: "rgba(255,255,255,0.12)", display: "grid", placeItems: "center" }}>
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#fff" strokeWidth="1.6"><circle cx="12" cy="13" r="4"/><path d="M3 8h4l2-3h6l2 3h4v12H3z"/></svg>
        </div>
      </div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// APPLE WATCH — custom frame + 3 screens
// ─────────────────────────────────────────────────────────────

const WatchFrame = ({ children }) => (
  <div style={{
    width: "100%", height: "100%",
    background: "#000",
    display: "grid", placeItems: "center",
    padding: 24,
  }}>
    <div style={{
      width: 184, height: 224,
      background: "#000",
      borderRadius: 38,
      border: "8px solid #2a2a2a",
      boxShadow: "0 30px 60px rgba(0,0,0,0.6), inset 0 0 0 1px #1a1a1a",
      position: "relative",
      overflow: "hidden",
    }}>
      {/* digital crown */}
      <div style={{ position: "absolute", right: -10, top: 56, width: 6, height: 28, background: "#3a3a3a", borderRadius: 3 }}/>
      {/* side button */}
      <div style={{ position: "absolute", right: -10, top: 130, width: 6, height: 18, background: "#3a3a3a", borderRadius: 3 }}/>
      <div style={{ width: "100%", height: "100%", color: "#fff", fontFamily: "var(--font-sans)" }}>
        {children}
      </div>
    </div>
  </div>
);

function WatchHome() {
  return (
    <WatchFrame>
      <div style={{ height: "100%", padding: 10, display: "flex", flexDirection: "column" }}>
        {/* time */}
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", fontSize: 9, marginBottom: 6 }}>
          <span style={{ color: "var(--on)", fontWeight: 600 }}>HomeHub</span>
          <span style={{ fontFamily: "var(--font-mono)", color: "var(--on)" }}>9:41</span>
        </div>
        {/* hero — whole home */}
        <div style={{ background: "linear-gradient(155deg, #2b2419, #221d14)", border: "1px solid rgba(245,189,110,0.18)", borderRadius: 14, padding: 8, display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 6 }}>
          <div>
            <div className="num-display" style={{ fontSize: 22, lineHeight: 1 }}>7</div>
            <div style={{ fontSize: 7.5, color: "var(--text-mute)" }}>of 23 on</div>
          </div>
          <div style={{ width: 26, height: 16, background: "var(--on)", borderRadius: 8, position: "relative" }}>
            <div style={{ position: "absolute", right: 2, top: 2, width: 12, height: 12, borderRadius: "50%", background: "#fff" }}/>
          </div>
        </div>
        {/* device list */}
        <div style={{ flex: 1, display: "flex", flexDirection: "column", gap: 4, overflow: "hidden" }}>
          {[
            { n: "Floor lamp", v: "62%", on: true },
            { n: "TV strip",   v: "28%", on: true },
            { n: "Porch",      v: "on",  on: true },
            { n: "Bedroom",    v: "off", on: false },
          ].map(d => (
            <div key={d.n} style={{ background: "#1c1a15", borderRadius: 8, padding: "5px 8px", display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
                <div style={{ width: 6, height: 6, borderRadius: "50%", background: d.on ? "var(--on)" : "#444", boxShadow: d.on ? "0 0 6px var(--on)" : "none" }}/>
                <span style={{ fontSize: 9, fontWeight: 500 }}>{d.n}</span>
              </div>
              <span style={{ fontSize: 9, fontFamily: "var(--font-mono)", color: d.on ? "var(--on)" : "#666" }}>{d.v}</span>
            </div>
          ))}
        </div>
      </div>
    </WatchFrame>
  );
}

function WatchScenes() {
  const scenes = [
    { n: "Evening",   c: "var(--on)",  active: true },
    { n: "Goodnight", c: "#7aa4d9" },
    { n: "Movie",     c: "#a96bd9" },
    { n: "Read",      c: "#d97a45" },
  ];
  return (
    <WatchFrame>
      <div style={{ height: "100%", padding: 10, display: "flex", flexDirection: "column" }}>
        <div style={{ fontSize: 9, color: "var(--on)", fontWeight: 600, marginBottom: 8 }}>Scenes</div>
        <div style={{ flex: 1, display: "grid", gridTemplateColumns: "1fr 1fr", gap: 6 }}>
          {scenes.map(s => (
            <div key={s.n} style={{ background: s.active ? "rgba(245,189,110,0.18)" : "#1c1a15", border: s.active ? "1px solid var(--on)" : "1px solid transparent", borderRadius: 12, padding: 8, display: "flex", flexDirection: "column", justifyContent: "space-between", aspectRatio: "1 / 1" }}>
              <div style={{ width: 16, height: 16, borderRadius: "50%", background: s.c, boxShadow: s.active ? "0 0 10px var(--on)" : "none", opacity: s.active ? 1 : 0.4 }}/>
              <div style={{ fontSize: 9, fontWeight: 600 }}>{s.n}</div>
            </div>
          ))}
        </div>
      </div>
    </WatchFrame>
  );
}

function WatchLight() {
  return (
    <WatchFrame>
      <div style={{ height: "100%", padding: 10, display: "flex", flexDirection: "column" }}>
        <div style={{ fontSize: 8.5, color: "var(--text-mute)", textAlign: "center", marginBottom: 4 }}>Floor lamp</div>

        {/* dial */}
        <div style={{ flex: 1, display: "grid", placeItems: "center", position: "relative" }}>
          <svg width="120" height="120" viewBox="0 0 120 120" style={{ transform: "rotate(-90deg)" }}>
            <circle cx="60" cy="60" r="50" fill="none" stroke="#222" strokeWidth="8"/>
            <circle cx="60" cy="60" r="50" fill="none" stroke="var(--on)" strokeWidth="8"
              strokeDasharray={`${0.62 * 2 * Math.PI * 50} ${2 * Math.PI * 50}`}
              strokeLinecap="round"
              style={{ filter: "drop-shadow(0 0 6px var(--on))" }}/>
          </svg>
          <div style={{ position: "absolute", textAlign: "center" }}>
            <div className="num-display" style={{ fontSize: 32, color: "var(--on)" }}>62</div>
            <div style={{ fontSize: 8, color: "var(--text-mute)", fontFamily: "var(--font-mono)" }}>%</div>
          </div>
        </div>

        {/* toggle pill */}
        <div style={{ display: "flex", gap: 4 }}>
          <div style={{ flex: 1, background: "var(--on)", color: "#1a1813", textAlign: "center", padding: "5px 0", borderRadius: 10, fontSize: 9, fontWeight: 600 }}>ON</div>
          <div style={{ width: 30, background: "#1c1a15", textAlign: "center", padding: "5px 0", borderRadius: 10, fontSize: 9, color: "var(--text-mute)" }}>···</div>
        </div>
      </div>
    </WatchFrame>
  );
}

// ─────────────────────────────────────────────────────────────
// DESKTOP — Insights
// ─────────────────────────────────────────────────────────────

function DesktopInsights() {
  const days = ["Mon","Tue","Wed","Thu","Fri","Sat","Sun"];
  const dayValues = [0.72, 0.95, 0.82, 0.88, 0.94, 0.62, 0.58];
  const hours = [0.04,0.04,0.03,0.03,0.03,0.05,0.10,0.18,0.22,0.15,0.10,0.12,
                 0.16,0.14,0.12,0.16,0.32,0.55,0.78,0.95,0.74,0.52,0.30,0.14];

  const nav = [
    { label: "Dashboard",  d: I.home },
    { label: "Rooms",      d: I.rooms },
    { label: "Devices",    d: I.bulb },
    { label: "Scenes",     d: I.scenes },
    { label: "Schedules",  d: I.schedule },
    { label: "Sensors",    d: I.sensor },
    { label: "Insights",   d: I.energy, active: true },
    { label: "Users",      d: I.user },
    { label: "Settings",   d: I.settings },
  ];

  return (
    <div className="hh" style={{ height: "100%", display: "flex", overflow: "hidden" }}>
      <aside className="nav-rail">
        <div style={{ display: "flex", alignItems: "center", gap: 10, padding: "4px 12px 22px" }}>
          <div style={{ width: 28, height: 28, borderRadius: 8, background: "var(--on)", display: "grid", placeItems: "center" }}>
            <div style={{ width: 12, height: 12, borderRadius: 3, background: "var(--bg)" }}/>
          </div>
          <div>
            <div style={{ fontSize: 15, fontWeight: 600, letterSpacing: "-0.02em" }}>HomeHub</div>
            <div style={{ fontSize: 10.5, color: "var(--text-mute)", fontFamily: "var(--font-mono)" }}>raspberrypi.local</div>
          </div>
        </div>
        {nav.map(n => (
          <div key={n.label} className={`nav-item ${n.active ? "active" : ""}`}>
            <Icon d={n.d} size={17} stroke={1.7}/>
            <span>{n.label}</span>
          </div>
        ))}
      </aside>

      <main style={{ flex: 1, padding: "28px 36px", overflow: "auto" }}>
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", marginBottom: 26 }}>
          <div>
            <div style={{ color: "var(--text-mute)", fontSize: 12.5, fontWeight: 500 }}>Energy & sensor history</div>
            <h1 style={{ fontSize: 30, fontWeight: 600, marginTop: 4, letterSpacing: "-0.03em" }}>Insights</h1>
          </div>
          <div style={{ display: "flex", gap: 6 }}>
            {["Today","Week","Month","Year","Custom"].map((r,i) => (
              <button key={r} className={`chip ${i===1?"active":""}`} style={{ padding: "8px 14px", fontSize: 12.5 }}>{r}</button>
            ))}
            <button className="chip" style={{ padding: "8px 14px", fontSize: 12.5 }}>Export CSV</button>
          </div>
        </div>

        {/* KPI row */}
        <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 14, marginBottom: 22 }}>
          {[
            { l: "kWh this week",   v: "23.4", sub: "−8% vs. last week",  c: "var(--on)",   pos: true },
            { l: "Estimated cost",  v: "$3.51", sub: "@ $0.15/kWh",        c: "var(--text)" },
            { l: "Peak day",        v: "Fri",   sub: "5.1 kWh — 19:00",    c: "var(--text)" },
            { l: "Always-on draw",  v: "42 W",  sub: "8 idle devices",     c: "var(--bad)" },
          ].map(k => (
            <div key={k.l} className="tile" style={{ padding: 18 }}>
              <div style={{ color: "var(--text-mute)", fontSize: 11, fontFamily: "var(--font-mono)", letterSpacing: "0.1em", textTransform: "uppercase" }}>{k.l}</div>
              <div style={{ fontSize: 32, fontWeight: 600, color: k.c, letterSpacing: "-0.02em", marginTop: 8 }}>{k.v}</div>
              <div style={{ color: k.pos ? "var(--good)" : "var(--text-mute)", fontSize: 12, marginTop: 4 }}>{k.sub}</div>
            </div>
          ))}
        </div>

        {/* main chart + sidebar */}
        <div style={{ display: "grid", gridTemplateColumns: "1.6fr 1fr", gap: 14, marginBottom: 22 }}>
          <div className="tile" style={{ padding: 22 }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", marginBottom: 18 }}>
              <div>
                <div style={{ color: "var(--text-mute)", fontSize: 11, fontFamily: "var(--font-mono)", letterSpacing: "0.1em", textTransform: "uppercase" }}>This week</div>
                <div style={{ fontSize: 22, fontWeight: 600, marginTop: 6 }}>Daily energy use</div>
              </div>
              <div style={{ display: "flex", gap: 8, fontSize: 11, color: "var(--text-mute)" }}>
                <span style={{ display: "flex", alignItems: "center", gap: 4 }}><span style={{ width: 8, height: 8, borderRadius: 2, background: "var(--on)" }}/>kWh</span>
                <span style={{ display: "flex", alignItems: "center", gap: 4 }}><span style={{ width: 8, height: 8, borderRadius: 2, background: "var(--card-3)" }}/>last week</span>
              </div>
            </div>
            <div style={{ display: "flex", alignItems: "flex-end", gap: 10, height: 200, padding: "0 4px" }}>
              {dayValues.map((v, i) => (
                <div key={i} style={{ flex: 1, display: "flex", flexDirection: "column", alignItems: "center", gap: 6 }}>
                  <div style={{ width: "100%", height: `${v*100}%`, position: "relative" }}>
                    <div style={{ position: "absolute", left: 0, right: "55%", bottom: 0, height: "100%", background: "var(--card-3)", borderRadius: "3px 3px 0 0" }}/>
                    <div style={{ position: "absolute", right: 0, left: "45%", bottom: 0, height: "100%", background: i===4?"var(--on)":"rgba(245,189,110,0.55)", borderRadius: "3px 3px 0 0", boxShadow: i===4 ? "0 0 16px var(--on-glow)" : "none" }}/>
                  </div>
                  <span style={{ fontSize: 11, color: i===4?"var(--on)":"var(--text-mute)", fontFamily: "var(--font-mono)" }}>{days[i]}</span>
                </div>
              ))}
            </div>
          </div>

          {/* donut by room */}
          <div className="tile" style={{ padding: 22 }}>
            <div style={{ color: "var(--text-mute)", fontSize: 11, fontFamily: "var(--font-mono)", letterSpacing: "0.1em", textTransform: "uppercase" }}>By room · this week</div>
            <div style={{ display: "flex", gap: 18, marginTop: 18, alignItems: "center" }}>
              <Donut
                size={140}
                stroke={20}
                segments={[
                  { v: 0.42, c: "#f5bd6e" },
                  { v: 0.28, c: "#84acc4" },
                  { v: 0.16, c: "#c4a4e0" },
                  { v: 0.09, c: "#9cc28a" },
                  { v: 0.05, c: "#e08a7a" },
                ]}
                center={
                  <div>
                    <div className="num-display" style={{ fontSize: 26 }}>23.4</div>
                    <div style={{ color: "var(--text-dim)", fontSize: 10, fontFamily: "var(--font-mono)" }}>kWh</div>
                  </div>
                }
              />
              <div style={{ flex: 1, display: "flex", flexDirection: "column", gap: 8 }}>
                {[
                  { c: "#f5bd6e", l: "Living room", v: "9.7" },
                  { c: "#84acc4", l: "Kitchen",     v: "6.4" },
                  { c: "#c4a4e0", l: "Bedroom",     v: "3.7" },
                  { c: "#9cc28a", l: "Outside",     v: "2.1" },
                  { c: "#e08a7a", l: "Hallway",     v: "1.5" },
                ].map(r => (
                  <div key={r.l} style={{ display: "flex", alignItems: "center", gap: 8 }}>
                    <div style={{ width: 8, height: 8, borderRadius: 2, background: r.c }}/>
                    <div style={{ flex: 1, fontSize: 12.5 }}>{r.l}</div>
                    <div className="mono" style={{ fontSize: 11.5, color: "var(--text-mute)" }}>{r.v}</div>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </div>

        {/* hourly profile */}
        <div className="tile" style={{ padding: 22 }}>
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline", marginBottom: 14 }}>
            <div>
              <div style={{ color: "var(--text-mute)", fontSize: 11, fontFamily: "var(--font-mono)", letterSpacing: "0.1em", textTransform: "uppercase" }}>Hourly profile · Tuesday</div>
              <div style={{ fontSize: 18, fontWeight: 600, marginTop: 4 }}>Peak at 19:00 — <span style={{ color: "var(--on)" }}>0.95 kWh</span></div>
            </div>
            <span style={{ color: "var(--text-mute)", fontSize: 12, fontFamily: "var(--font-mono)" }}>24h · 1h buckets</span>
          </div>
          <SparkBars data={hours} peakIdx={19} height={120}/>
        </div>
      </main>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// DESKTOP — Automations
// ─────────────────────────────────────────────────────────────

function DesktopAutomations() {
  const nav = [
    { label: "Dashboard",  d: I.home },
    { label: "Rooms",      d: I.rooms },
    { label: "Devices",    d: I.bulb },
    { label: "Scenes",     d: I.scenes },
    { label: "Schedules",  d: I.schedule },
    { label: "Automations",d: I.sliders, active: true },
    { label: "Sensors",    d: I.sensor },
    { label: "Insights",   d: I.energy },
    { label: "Users",      d: I.user },
    { label: "Settings",   d: I.settings },
  ];

  return (
    <div className="hh" style={{ height: "100%", display: "flex", overflow: "hidden" }}>
      <aside className="nav-rail">
        <div style={{ display: "flex", alignItems: "center", gap: 10, padding: "4px 12px 22px" }}>
          <div style={{ width: 28, height: 28, borderRadius: 8, background: "var(--on)", display: "grid", placeItems: "center" }}>
            <div style={{ width: 12, height: 12, borderRadius: 3, background: "var(--bg)" }}/>
          </div>
          <div>
            <div style={{ fontSize: 15, fontWeight: 600 }}>HomeHub</div>
            <div style={{ fontSize: 10.5, color: "var(--text-mute)", fontFamily: "var(--font-mono)" }}>raspberrypi.local</div>
          </div>
        </div>
        {nav.map(n => (
          <div key={n.label} className={`nav-item ${n.active ? "active" : ""}`}>
            <Icon d={n.d} size={17} stroke={1.7}/>
            <span>{n.label}</span>
          </div>
        ))}
      </aside>

      <main style={{ flex: 1, padding: "28px 36px", overflow: "auto", display: "flex", flexDirection: "column", gap: 22 }}>
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}>
          <div>
            <div style={{ color: "var(--text-mute)", fontSize: 12.5 }}>WHEN this, THEN that</div>
            <h1 style={{ fontSize: 30, fontWeight: 600, marginTop: 4, letterSpacing: "-0.03em" }}>Automations</h1>
          </div>
          <div style={{ display: "flex", gap: 8 }}>
            <button className="chip" style={{ padding: "9px 12px", fontSize: 13 }}>Import YAML</button>
            <button className="chip" style={{ padding: "9px 14px", fontSize: 13, background: "var(--on)", color: "#1a1813", borderColor: "var(--on)" }}>
              <Icon d={I.plus} size={14} stroke={2.2}/> New automation
            </button>
          </div>
        </div>

        {/* split: list + builder preview */}
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1.4fr", gap: 14, flex: 1, minHeight: 0 }}>
          {/* list */}
          <div className="tile" style={{ padding: 0, overflow: "hidden", display: "flex", flexDirection: "column" }}>
            <div style={{ padding: "14px 18px", borderBottom: "1px solid var(--hairline)", display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <span style={{ fontSize: 12, color: "var(--text-mute)", fontFamily: "var(--font-mono)", letterSpacing: "0.08em", textTransform: "uppercase" }}>7 automations</span>
              <Icon d={I.search} size={14} stroke={1.7} style={{ color: "var(--text-dim)" }}/>
            </div>
            {[
              { n: "Wake gradient",         w: "06:45 weekdays",    on: false, active: true },
              { n: "Sunset porch",          w: "At sunset",          on: true,  runs: 184 },
              { n: "Goodnight",             w: "23:00 weekdays",     on: true,  runs: 96 },
              { n: "Motion — hallway",      w: "Motion · after dark", on: true, runs: 412 },
              { n: "Away — no one home",    w: "Everyone left",      on: true,  runs: 12 },
              { n: "Movie mode",            w: "TV turns on",        on: true,  runs: 27 },
              { n: "Coffee bar",            w: "06:30 weekdays",     on: false, runs: 0 },
            ].map(a => (
              <div key={a.n} style={{ padding: "12px 18px", borderBottom: "1px solid var(--hairline)", background: a.active ? "var(--card-2)" : "transparent", display: "flex", alignItems: "center", gap: 12, cursor: "pointer" }}>
                <span className="dot" style={{ background: a.on ? "var(--on)" : "var(--text-dim)", boxShadow: a.on ? "0 0 0 4px var(--on-soft)" : "none" }}/>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ fontSize: 13.5, fontWeight: 500 }}>{a.n}</div>
                  <div style={{ color: "var(--text-mute)", fontSize: 11.5, fontFamily: "var(--font-mono)", marginTop: 2 }}>{a.w}</div>
                </div>
                <div className={`sw ${a.on ? "on" : ""}`}/>
              </div>
            ))}
          </div>

          {/* builder preview */}
          <div className="tile" style={{ padding: 22, overflow: "auto" }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 18 }}>
              <div>
                <div style={{ color: "var(--text-mute)", fontSize: 11, fontFamily: "var(--font-mono)", letterSpacing: "0.1em", textTransform: "uppercase" }}>Editing</div>
                <h2 style={{ fontSize: 22, fontWeight: 600, marginTop: 4 }}>Wake gradient</h2>
              </div>
              <div style={{ display: "flex", gap: 8 }}>
                <button className="chip" style={{ padding: "7px 12px", fontSize: 12 }}>Test run</button>
                <button className="chip" style={{ padding: "7px 12px", fontSize: 12, color: "var(--bad)" }}>Delete</button>
              </div>
            </div>

            {/* WHEN node */}
            <div style={{ display: "flex", gap: 14, marginBottom: 18 }}>
              <div style={{ width: 50, display: "flex", flexDirection: "column", alignItems: "center" }}>
                <div style={{ width: 50, height: 26, borderRadius: 13, background: "var(--cool-soft)", color: "var(--cool)", display: "grid", placeItems: "center", fontSize: 10, fontFamily: "var(--font-mono)", letterSpacing: "0.06em", textTransform: "uppercase", fontWeight: 600 }}>When</div>
                <div style={{ flex: 1, width: 1, background: "var(--hairline)", marginTop: 4 }}/>
              </div>
              <div style={{ flex: 1, background: "var(--card-2)", borderRadius: 14, padding: 16, border: "1px solid var(--hairline)", borderLeft: "3px solid var(--cool)" }}>
                <div style={{ fontSize: 13.5, fontWeight: 500, marginBottom: 4 }}>Time of day</div>
                <div style={{ display: "flex", gap: 16, alignItems: "center" }}>
                  <span className="mono" style={{ fontSize: 28, color: "var(--text)", letterSpacing: "-0.02em" }}>06:45</span>
                  <div style={{ display: "flex", gap: 4 }}>
                    {["M","T","W","T","F","S","S"].map((d,i) => (
                      <div key={i} style={{ width: 28, height: 28, borderRadius: 8, background: i<5 ? "var(--on-soft)" : "var(--card-3)", color: i<5 ? "var(--on)" : "var(--text-dim)", display: "grid", placeItems: "center", fontSize: 11, fontWeight: 600 }}>{d}</div>
                    ))}
                  </div>
                </div>
              </div>
            </div>

            {/* AND IF (optional) */}
            <div style={{ display: "flex", gap: 14, marginBottom: 18 }}>
              <div style={{ width: 50, display: "flex", flexDirection: "column", alignItems: "center" }}>
                <div style={{ width: 50, height: 26, borderRadius: 13, background: "var(--card-3)", color: "var(--text-mute)", display: "grid", placeItems: "center", fontSize: 10, fontFamily: "var(--font-mono)", letterSpacing: "0.06em", textTransform: "uppercase", fontWeight: 600 }}>If</div>
                <div style={{ flex: 1, width: 1, background: "var(--hairline)", marginTop: 4 }}/>
              </div>
              <div style={{ flex: 1, background: "transparent", borderRadius: 14, padding: 14, border: "1px dashed var(--border)", color: "var(--text-mute)", fontSize: 12.5 }}>
                + Add condition (optional) — e.g. sunrise &lt; 07:00, or sensor reading
              </div>
            </div>

            {/* THEN nodes */}
            <div style={{ display: "flex", gap: 14 }}>
              <div style={{ width: 50, display: "flex", flexDirection: "column", alignItems: "center" }}>
                <div style={{ width: 50, height: 26, borderRadius: 13, background: "var(--on-soft)", color: "var(--on)", display: "grid", placeItems: "center", fontSize: 10, fontFamily: "var(--font-mono)", letterSpacing: "0.06em", textTransform: "uppercase", fontWeight: 600 }}>Then</div>
              </div>
              <div style={{ flex: 1, display: "flex", flexDirection: "column", gap: 10 }}>
                {[
                  { l: "Bedroom main", v: "Fade 10% → 100% over 8 min", icon: I.bulb },
                  { l: "Coffee bar",   v: "Turn on",                   icon: I.bulb },
                  { l: "Open blinds",  v: "Wait 4 min, then open",     icon: I.sliders },
                ].map((a, i) => (
                  <div key={i} style={{ background: "var(--card-2)", borderRadius: 14, padding: 14, border: "1px solid var(--hairline)", borderLeft: "3px solid var(--on)", display: "flex", alignItems: "center", gap: 14 }}>
                    <div style={{ width: 36, height: 36, borderRadius: 10, background: "var(--card-3)", display: "grid", placeItems: "center", color: "var(--on)" }}>
                      <Icon d={a.icon} size={16} stroke={1.8}/>
                    </div>
                    <div style={{ flex: 1 }}>
                      <div style={{ fontSize: 13.5, fontWeight: 500 }}>{a.l}</div>
                      <div style={{ color: "var(--text-mute)", fontSize: 11.5, fontFamily: "var(--font-mono)" }}>{a.v}</div>
                    </div>
                    <Icon d={I.more} size={14} stroke={2} style={{ color: "var(--text-dim)" }}/>
                  </div>
                ))}
                <button style={{ background: "transparent", border: "1px dashed var(--border)", borderRadius: 14, padding: 12, color: "var(--on)", fontSize: 12.5, textAlign: "left" }}>
                  + Add action
                </button>
              </div>
            </div>
          </div>
        </div>
      </main>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// DESKTOP — Activity
// ─────────────────────────────────────────────────────────────

function DesktopActivity() {
  const events = [
    { t: "19:42", who: "Auto",  what: "Sunset porch",     det: "Porch on · 80%",                   k: "auto" },
    { t: "19:01", who: "Mira",  what: "Evening scene",     det: "8 devices changed",                k: "scene" },
    { t: "18:14", who: "Auto",  what: "Motion — hallway", det: "Hallway 30% for 2 min",            k: "auto" },
    { t: "12:03", who: "Mira",  what: "Kitchen isle",     det: "Brightness 80 → 100%",             k: "device" },
    { t: "06:31", who: "Auto",  what: "Coffee bar",       det: "Coffee bar on",                    k: "auto" },
    { t: "23:01", who: "Auto",  what: "Goodnight",        det: "All off · Door locked",            k: "auto", day: "Yesterday" },
    { t: "22:18", who: "Dad",   what: "Bedroom main",     det: "Turned off via Watch",             k: "device" },
    { t: "18:42", who: "Mira",  what: "Evening scene",     det: "8 devices changed",                k: "scene" },
    { t: "14:30", who: "Hub",   what: "Floor lamp",       det: "Reconnected · was offline 4 min",  k: "system" },
  ];
  const kindStyle = {
    auto:   { c: "var(--on)",   d: I.scenes },
    scene:  { c: "#c4a4e0",     d: I.star },
    device: { c: "var(--cool)", d: I.bulb },
    system: { c: "var(--text-dim)", d: I.settings },
  };

  const nav = [
    { label: "Dashboard",  d: I.home },
    { label: "Rooms",      d: I.rooms },
    { label: "Devices",    d: I.bulb },
    { label: "Scenes",     d: I.scenes },
    { label: "Schedules",  d: I.schedule },
    { label: "Sensors",    d: I.sensor },
    { label: "Insights",   d: I.energy },
    { label: "Activity",   d: I.bell, active: true },
    { label: "Users",      d: I.user },
    { label: "Settings",   d: I.settings },
  ];

  return (
    <div className="hh" style={{ height: "100%", display: "flex", overflow: "hidden" }}>
      <aside className="nav-rail">
        <div style={{ display: "flex", alignItems: "center", gap: 10, padding: "4px 12px 22px" }}>
          <div style={{ width: 28, height: 28, borderRadius: 8, background: "var(--on)", display: "grid", placeItems: "center" }}>
            <div style={{ width: 12, height: 12, borderRadius: 3, background: "var(--bg)" }}/>
          </div>
          <div>
            <div style={{ fontSize: 15, fontWeight: 600 }}>HomeHub</div>
            <div style={{ fontSize: 10.5, color: "var(--text-mute)", fontFamily: "var(--font-mono)" }}>raspberrypi.local</div>
          </div>
        </div>
        {nav.map(n => (
          <div key={n.label} className={`nav-item ${n.active ? "active" : ""}`}>
            <Icon d={n.d} size={17} stroke={1.7}/>
            <span>{n.label}</span>
          </div>
        ))}
      </aside>

      <main style={{ flex: 1, padding: "28px 36px", overflow: "auto" }}>
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", marginBottom: 22 }}>
          <div>
            <div style={{ color: "var(--text-mute)", fontSize: 12.5 }}>Every command, every trigger</div>
            <h1 style={{ fontSize: 30, fontWeight: 600, marginTop: 4, letterSpacing: "-0.03em" }}>Activity</h1>
          </div>
          <div style={{ display: "flex", gap: 6 }}>
            {["All","Automations","People","Devices","System"].map((t,i) => (
              <button key={t} className={`chip ${i===0?"active":""}`} style={{ padding: "8px 14px", fontSize: 12.5 }}>{t}</button>
            ))}
          </div>
        </div>

        {/* table-style log */}
        <div className="tile" style={{ padding: 0, overflow: "hidden" }}>
          {/* head */}
          <div style={{ display: "grid", gridTemplateColumns: "92px 1fr 1fr 1.5fr 80px", padding: "12px 22px", color: "var(--text-mute)", fontSize: 11, fontFamily: "var(--font-mono)", letterSpacing: "0.08em", textTransform: "uppercase", borderBottom: "1px solid var(--hairline)" }}>
            <span>Time</span><span>Kind</span><span>Actor</span><span>Detail</span><span>Source</span>
          </div>
          {events.map((e, i) => {
            const k = kindStyle[e.k];
            return (
              <React.Fragment key={i}>
                {e.day && (
                  <div style={{ padding: "10px 22px", background: "var(--card-2)", color: "var(--text-mute)", fontSize: 11, fontFamily: "var(--font-mono)", letterSpacing: "0.08em", textTransform: "uppercase" }}>{e.day}</div>
                )}
                <div style={{ display: "grid", gridTemplateColumns: "92px 1fr 1fr 1.5fr 80px", padding: "14px 22px", borderBottom: i < events.length-1 ? "1px solid var(--hairline)" : "none", alignItems: "center", fontSize: 13 }}>
                  <span className="mono" style={{ color: "var(--text-mute)", fontSize: 12 }}>{e.t}</span>
                  <span style={{ display: "flex", alignItems: "center", gap: 8, color: k.c }}>
                    <Icon d={k.d} size={14} stroke={1.8}/>
                    {e.k}
                  </span>
                  <span style={{ fontWeight: 500 }}>{e.who}</span>
                  <span><span style={{ fontWeight: 500 }}>{e.what}</span> <span style={{ color: "var(--text-mute)" }}>— {e.det}</span></span>
                  <span className="mono" style={{ color: "var(--text-dim)", fontSize: 11 }}>{e.k === "device" ? "API" : e.k === "system" ? "MQTT" : "rule"}</span>
                </div>
              </React.Fragment>
            );
          })}
        </div>
      </main>
    </div>
  );
}

// expose to other files
Object.assign(window, {
  InsightsScreen,
  AutomationsScreen,
  AutomationBuilderScreen,
  ActivityScreen,
  IOSLockScreen,
  WatchHome, WatchScenes, WatchLight,
  DesktopInsights, DesktopAutomations, DesktopActivity,
});
