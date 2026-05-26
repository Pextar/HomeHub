/* HomeHub — list screens: Devices, Groups, Sensors, Users, Floor plan, Settings. */

// ── DEVICES (full list) ────────────────────────────────────
function DevicesScreen() {
  const groups = [
    { room: "Living room", devices: [
      { name: "Floor lamp",   on: true,  dim: 62,  protocol: "matter" },
      { name: "TV strip",     on: true,  dim: 28,  protocol: "matter" },
      { name: "Sofa lamp",    on: false, protocol: "rf" },
      { name: "Reading nook", on: false, protocol: "matter" },
      { name: "Window",       on: true,  protocol: "rf" },
    ]},
    { room: "Kitchen", devices: [
      { name: "Kitchen isle",  on: true,  dim: 100, protocol: "wifi" },
      { name: "Under cabinet", on: true,  dim: 80,  protocol: "wifi" },
      { name: "Coffee bar",    on: false, protocol: "rf" },
    ]},
    { room: "Bedroom", devices: [
      { name: "Bedroom main",  on: false, protocol: "matter" },
      { name: "Nightstand L",  on: false, protocol: "matter" },
      { name: "Nightstand R",  on: false, protocol: "matter" },
    ]},
  ];

  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "hidden", paddingBottom: 90 }}>
      <div style={{ height: 54 }}/>

      <div style={{ padding: `8px 22px 0`, display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <h1 style={{ fontSize: 30, fontWeight: 600, letterSpacing: "-0.03em" }}>Devices</h1>
        <button className="chip" style={{ padding: "8px 14px", fontSize: 13 }}>
          <Icon d={I.plus} size={14} stroke={2}/> Add
        </button>
      </div>
      <div style={{ padding: `2px 22px 0`, color: "var(--text-mute)", fontSize: 13 }}>23 devices · 7 on</div>

      {/* search */}
      <div style={{ padding: "16px 22px 0" }}>
        <div className="card" style={{ padding: "12px 14px", flexDirection: "row", display: "flex", alignItems: "center", gap: 10 }}>
          <Icon d={I.search} size={16} stroke={1.7} style={{ color: "var(--text-mute)" }}/>
          <span style={{ color: "var(--text-dim)", fontSize: 14 }}>Search devices…</span>
        </div>
      </div>

      {/* filter chips */}
      <div className="h-scroll" style={{ marginTop: 14 }}>
        {[
          ["All", true], ["On", false], ["Lights", false], ["Plugs", false],
          ["Matter", false], ["Wi-Fi", false], ["RF", false],
        ].map(([c, a]) => (
          <button key={c} className={`chip ${a ? "active" : ""}`}>{c}</button>
        ))}
      </div>

      {groups.map(g => (
        <React.Fragment key={g.room}>
          <SectionHead title={g.room} right={
            <span style={{ color: "var(--text-mute)", fontSize: 12.5 }}>
              <span className="mono" style={{ color: "var(--on)" }}>
                {g.devices.filter(d => d.on).length}
              </span>
              <span style={{ color: "var(--text-dim)" }}> / {g.devices.length}</span>
            </span>
          }/>
          <div style={{ padding: "0 22px", display: "flex", flexDirection: "column", gap: 6 }}>
            {g.devices.map(d => (
              <div key={d.name} className="card" style={{ padding: 12, flexDirection: "row", display: "flex", alignItems: "center", gap: 12 }}>
                <div className="tile-bulb" style={{ width: 32, height: 32, background: d.on ? "var(--on)" : "var(--card-3)", boxShadow: d.on ? `0 0 16px var(--on-glow)` : "none" }}>
                  <Icon d={I.bulb} size={15} stroke={1.7} style={{ color: d.on ? "#3a2400" : "var(--text-mute)" }}/>
                </div>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ fontSize: 14, fontWeight: 500 }}>{d.name}</div>
                  <div style={{ display: "flex", gap: 8, alignItems: "center", marginTop: 2 }}>
                    <span style={{ color: "var(--text-mute)", fontSize: 12 }}>{d.on ? (d.dim != null ? `On · ${d.dim}%` : "On") : "Off"}</span>
                    <ProtocolBadge kind={d.protocol}/>
                  </div>
                </div>
                <div className={`sw ${d.on ? "on" : ""}`}/>
              </div>
            ))}
          </div>
        </React.Fragment>
      ))}

      <TabBar active="rooms"/>
    </div>
  );
}

// ── GROUPS ─────────────────────────────────────────────────
function GroupsScreen() {
  const groups = [
    { name: "Downstairs",       count: 7, members: ["Floor lamp", "TV strip", "Sofa lamp", "Kitchen isle"], on: 4 },
    { name: "All overhead",     count: 6, members: ["Bedroom main", "Hallway", "Kitchen isle"], on: 2 },
    { name: "Bedside",          count: 3, members: ["Nightstand L", "Nightstand R", "Bedroom main"], on: 0 },
    { name: "Outdoors",         count: 5, members: ["Porch", "Garage", "Garden", "Path lights"], on: 1 },
  ];
  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "hidden", paddingBottom: 90 }}>
      <div style={{ height: 54 }}/>

      <div style={{ padding: `8px 22px 0`, display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <h1 style={{ fontSize: 30, fontWeight: 600, letterSpacing: "-0.03em" }}>Groups</h1>
        <button className="chip" style={{ padding: "8px 14px", fontSize: 13 }}>
          <Icon d={I.plus} size={14} stroke={2}/> New group
        </button>
      </div>
      <div style={{ padding: `2px 22px 0`, color: "var(--text-mute)", fontSize: 13 }}>Control multiple devices at once</div>

      <div style={{ padding: "22px 22px 0", display: "flex", flexDirection: "column", gap: 10 }}>
        {groups.map(g => (
          <div key={g.name} className={`tile ${g.on > 0 ? "on" : ""}`} style={{ padding: 16, gap: 12 }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}>
              <div style={{ width: 36, height: 36, borderRadius: 10, background: g.on > 0 ? "var(--on)" : "var(--card-3)", display: "grid", placeItems: "center" }}>
                <Icon d={I.group} size={17} stroke={1.7} style={{ color: g.on > 0 ? "#3a2400" : "var(--text-mute)" }}/>
              </div>
              <div className={`sw ${g.on > 0 ? "on" : ""}`}/>
            </div>
            <div>
              <div style={{ fontWeight: 600, fontSize: 16 }}>{g.name}</div>
              <div style={{ color: "var(--text-mute)", fontSize: 12.5, marginTop: 3 }}>
                <span className="mono" style={{ color: g.on > 0 ? "var(--on)" : "var(--text-mute)" }}>{g.on}</span>
                <span style={{ color: "var(--text-dim)" }}> / {g.count}</span> on · {g.members.slice(0,2).join(", ")}{g.members.length > 2 ? ` +${g.members.length - 2}` : ""}
              </div>
            </div>
          </div>
        ))}
      </div>

      <TabBar active="rooms"/>
    </div>
  );
}

// ── SENSORS ─────────────────────────────────────────────────
function SensorsScreen() {
  const sensors = [
    { name: "Living room", kind: "temp", value: 21.4, unit: "°C", spark: [20.8,20.9,21.1,21.0,21.2,21.4,21.4], alert: false },
    { name: "Living room", kind: "humid", value: 42, unit: "%", spark: [38,39,40,41,42,42,42], alert: false },
    { name: "Bedroom",    kind: "temp", value: 17.6, unit: "°C", spark: [19,18.6,18.4,18.0,17.8,17.6,17.6], alert: true,  alertText: "Below 18°" },
    { name: "Outside",    kind: "temp", value: 12.0, unit: "°C", spark: [13,12.8,12.5,12.3,12.1,12.0,12.0], alert: false },
    { name: "Hallway",    kind: "motion", value: "Idle", spark: null, alert: false, last: "12 min ago" },
    { name: "Garage",     kind: "power", value: 184, unit: "W", spark: [120,140,160,170,180,184,184], alert: false },
  ];

  const kindIcon = {
    temp: I.thermo, humid: I.drop, motion: I.motion, power: I.energy, light: I.sun, custom: I.sensor,
  };

  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "hidden", paddingBottom: 90 }}>
      <div style={{ height: 54 }}/>

      <div style={{ padding: `8px 22px 0`, display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <h1 style={{ fontSize: 30, fontWeight: 600, letterSpacing: "-0.03em" }}>Sensors</h1>
        <button className="chip" style={{ padding: "8px 14px", fontSize: 13 }}>
          <Icon d={I.plus} size={14} stroke={2}/> Pair
        </button>
      </div>
      <div style={{ padding: `2px 22px 0`, color: "var(--text-mute)", fontSize: 13 }}>6 sensors · <span style={{ color: "var(--bad)" }}>1 alert</span></div>

      <div style={{ padding: "22px 22px 0", display: "grid", gridTemplateColumns: "1fr 1fr", gap: 10 }}>
        {sensors.map(s => (
          <div key={s.name + s.kind} className="card" style={{ padding: 14, gap: 8, borderColor: s.alert ? "var(--bad)" : "var(--hairline)" }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <Icon d={kindIcon[s.kind]} size={16} stroke={1.7} style={{ color: s.alert ? "var(--bad)" : "var(--cool)" }}/>
              {s.alert && <span style={{ fontFamily: "var(--font-mono)", fontSize: 10, color: "var(--bad)", fontWeight: 500 }}>ALERT</span>}
            </div>
            <div>
              <div style={{ fontWeight: 600, fontSize: 14 }}>{s.name}</div>
              <div style={{ color: "var(--text-mute)", fontSize: 11, marginTop: 1, textTransform: "capitalize" }}>{s.kind === "humid" ? "Humidity" : s.kind}</div>
            </div>
            <div style={{ display: "flex", alignItems: "baseline", gap: 4, marginTop: 4 }}>
              <span className="num-display" style={{ fontSize: 26, color: s.alert ? "var(--bad)" : "var(--text)" }}>{s.value}</span>
              {s.unit && <span style={{ color: "var(--text-mute)", fontSize: 12 }}>{s.unit}</span>}
            </div>
            {/* sparkline */}
            {s.spark && (
              <svg viewBox="0 0 100 20" style={{ width: "100%", height: 22, marginTop: 2 }} preserveAspectRatio="none">
                {(() => {
                  const min = Math.min(...s.spark), max = Math.max(...s.spark), range = max - min || 1;
                  const pts = s.spark.map((v, i) => `${(i / (s.spark.length - 1)) * 100},${20 - ((v - min) / range) * 18 - 1}`).join(" ");
                  return <polyline points={pts} fill="none" stroke={s.alert ? "var(--bad)" : "var(--cool)"} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/>;
                })()}
              </svg>
            )}
            {!s.spark && s.last && (
              <div style={{ color: "var(--text-dim)", fontSize: 11, marginTop: 4 }}>Last: {s.last}</div>
            )}
          </div>
        ))}
      </div>

      <TabBar active="rooms"/>
    </div>
  );
}

// ── USERS (admin) ─────────────────────────────────────────
function UsersScreen() {
  const users = [
    { name: "Mira",   role: "Admin",   sub: "All devices · password", initial: "M", color: "var(--on)" },
    { name: "Theo",   role: "Limited", sub: "4 devices · code 5029",  initial: "T", color: "var(--cool)" },
    { name: "Alex",   role: "Limited", sub: "12 devices · code 7142", initial: "A", color: "#a96bd9" },
    { name: "Cleaner", role: "Limited", sub: "2 devices · code 3380", initial: "C", color: "var(--good)" },
    { name: "Kids",   role: "Kid mode", sub: "2 lamps · code 9011",   initial: "K", color: "#d97a45" },
  ];

  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "hidden", paddingBottom: 90 }}>
      <div style={{ height: 54 }}/>

      <div style={{ padding: `8px 22px 0`, display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <h1 style={{ fontSize: 30, fontWeight: 600, letterSpacing: "-0.03em" }}>Users</h1>
        <button className="chip" style={{ padding: "8px 14px", fontSize: 13 }}>
          <Icon d={I.plus} size={14} stroke={2}/> Invite
        </button>
      </div>
      <div style={{ padding: `2px 22px 0`, color: "var(--text-mute)", fontSize: 13 }}>5 profiles · 1 admin</div>

      <div style={{ padding: "22px 22px 0", display: "flex", flexDirection: "column", gap: 8 }}>
        {users.map(u => (
          <div key={u.name} className="card" style={{ padding: 14, flexDirection: "row", display: "flex", alignItems: "center", gap: 12 }}>
            <div style={{ width: 42, height: 42, borderRadius: "50%", background: u.color, display: "grid", placeItems: "center", color: "#3a2400", fontWeight: 600, fontSize: 16, fontFamily: "var(--font-mono)", flexShrink: 0 }}>
              {u.initial}
            </div>
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                <span style={{ fontWeight: 600, fontSize: 15 }}>{u.name}</span>
                {u.role === "Admin" && <span style={{ fontSize: 10, fontFamily: "var(--font-mono)", padding: "2px 6px", borderRadius: 6, background: "var(--on-soft)", color: "var(--on)", letterSpacing: "0.04em" }}>ADMIN</span>}
                {u.role === "Kid mode" && <span style={{ fontSize: 10, fontFamily: "var(--font-mono)", padding: "2px 6px", borderRadius: 6, background: "var(--card-3)", color: "var(--text-mute)", letterSpacing: "0.04em" }}>KID</span>}
              </div>
              <div style={{ color: "var(--text-mute)", fontSize: 12.5, marginTop: 2 }}>{u.sub}</div>
            </div>
            <Icon d={I.chevR} size={14} stroke={1.7} style={{ color: "var(--text-dim)" }}/>
          </div>
        ))}
      </div>

      <SectionHead title="Pending invites"/>
      <div style={{ padding: "0 22px" }}>
        <div className="card" style={{ padding: 14, flexDirection: "row", display: "flex", alignItems: "center", gap: 12 }}>
          <div style={{ width: 42, height: 42, borderRadius: "50%", background: "var(--card-3)", display: "grid", placeItems: "center", border: "1.5px dashed var(--border)", color: "var(--text-mute)", fontFamily: "var(--font-mono)" }}>
            ?
          </div>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ fontWeight: 600, fontSize: 15 }}>guest@home</div>
            <div style={{ color: "var(--text-mute)", fontSize: 12.5, marginTop: 2 }}>Invited 2 days ago · expires in 5d</div>
          </div>
          <button style={{ color: "var(--cool)", fontSize: 13, fontWeight: 500 }}>Resend</button>
        </div>
      </div>

      <TabBar active="rooms"/>
    </div>
  );
}

// ── SETTINGS ─────────────────────────────────────────────────
function SettingsScreen() {
  const Row = ({ d, label, value, danger, last }) => (
    <>
      <div style={{ display: "flex", alignItems: "center", padding: "14px 16px", gap: 12 }}>
        {d && <Icon d={d} size={17} stroke={1.7} style={{ color: danger ? "var(--bad)" : "var(--text-mute)" }}/>}
        <span style={{ fontSize: 14, fontWeight: 500, color: danger ? "var(--bad)" : "var(--text)" }}>{label}</span>
        <span style={{ marginLeft: "auto", color: "var(--text-mute)", fontSize: 13, fontFamily: typeof value === "string" && value.match(/\d/) ? "var(--font-mono)" : "inherit" }}>{value}</span>
        {!danger && <Icon d={I.chevR} size={14} stroke={1.7} style={{ color: "var(--text-dim)" }}/>}
      </div>
      {!last && <div className="sep" style={{ marginLeft: 50 }}/>}
    </>
  );

  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "hidden", paddingBottom: 90 }}>
      <div style={{ height: 54 }}/>

      <div style={{ padding: `8px 22px 0` }}>
        <h1 style={{ fontSize: 30, fontWeight: 600, letterSpacing: "-0.03em" }}>Settings</h1>
      </div>

      {/* admin profile card */}
      <div style={{ padding: "20px 22px 0" }}>
        <div className="card" style={{ padding: 16, flexDirection: "row", display: "flex", alignItems: "center", gap: 14 }}>
          <div style={{ width: 50, height: 50, borderRadius: "50%", background: "var(--on)", display: "grid", placeItems: "center", color: "#3a2400", fontWeight: 600, fontSize: 18, fontFamily: "var(--font-mono)" }}>
            M
          </div>
          <div style={{ flex: 1 }}>
            <div style={{ fontWeight: 600, fontSize: 16 }}>Mira</div>
            <div style={{ color: "var(--text-mute)", fontSize: 12.5 }}>Admin · signed in</div>
          </div>
          <button style={{ color: "var(--text-mute)", fontSize: 13, padding: "8px 12px", borderRadius: 10, border: "1px solid var(--hairline)" }}>Edit</button>
        </div>
      </div>

      <SectionHead title="Home"/>
      <div style={{ padding: "0 22px" }}>
        <div className="card" style={{ padding: 0, overflow: "hidden" }}>
          <Row d={I.home}     label="Home name"     value="HomeHub"/>
          <Row d={I.rooms}    label="Rooms"         value="6"/>
          <Row d={I.sun}      label="Location"      value="Stockholm"/>
          <Row d={I.bell}     label="Notifications" value="On" last/>
        </div>
      </div>

      <SectionHead title="System"/>
      <div style={{ padding: "0 22px" }}>
        <div className="card" style={{ padding: 0, overflow: "hidden" }}>
          <Row d={I.wifi}     label="Network"   value="raspberrypi.local"/>
          <Row d={I.matter}   label="Matter bridge" value="Running"/>
          <Row d={I.rf}       label="433 MHz radio" value="rpi-rf · OK"/>
          <Row d={I.energy}   label="MQTT broker"   value="Disabled"/>
          <Row d={I.scenes}   label="Console"       value="Live" last/>
        </div>
      </div>

      <SectionHead title="App"/>
      <div style={{ padding: "0 22px" }}>
        <div className="card" style={{ padding: 0, overflow: "hidden" }}>
          <Row d={I.settings} label="Theme"      value="Dark"/>
          <Row d={I.sun}      label="Reduce motion" value="System"/>
          <Row d={I.settings} label="About"      value="v2.4.1" last/>
        </div>
      </div>

      <div style={{ padding: "22px 22px 0" }}>
        <div className="card" style={{ padding: 0, overflow: "hidden" }}>
          <Row d={I.power}    label="Sign out"   value="" danger last/>
        </div>
      </div>

      <div style={{ height: 32 }}/>
      <TabBar active="settings"/>
    </div>
  );
}

// ── FLOOR PLAN ───────────────────────────────────────────
function FloorPlanScreen() {
  // Simple wireframe-ish layout — abstract room blocks with device dots
  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "hidden", paddingBottom: 90 }}>
      <div style={{ height: 54 }}/>

      <div style={{ padding: `8px 22px 0`, display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <h1 style={{ fontSize: 30, fontWeight: 600, letterSpacing: "-0.03em" }}>Floor plan</h1>
        <button className="chip" style={{ padding: "8px 14px", fontSize: 13 }}>
          <Icon d={I.settings} size={14} stroke={1.7}/> Edit
        </button>
      </div>

      <div style={{ padding: "18px 22px 0" }}>
        <div className="card" style={{ padding: 16, height: 420, position: "relative", overflow: "hidden", background: "var(--card)" }}>
          {/* faint dotted grid */}
          <div style={{ position: "absolute", inset: 0, backgroundImage: "radial-gradient(circle, var(--hairline) 1px, transparent 1px)", backgroundSize: "16px 16px", opacity: 0.6 }}/>

          {/* rooms as shapes */}
          {[
            { x: 6, y: 6, w: 50, h: 38, name: "Living room", on: 3, c: "warm" },
            { x: 58, y: 6, w: 36, h: 30, name: "Kitchen", on: 2, c: "warm" },
            { x: 58, y: 38, w: 36, h: 28, name: "Bath", on: 0, c: "cool" },
            { x: 6, y: 46, w: 30, h: 28, name: "Bedroom", on: 0, c: "cool" },
            { x: 38, y: 68, w: 28, h: 22, name: "Hall", on: 1, c: "neutral" },
            { x: 38, y: 46, w: 18, h: 20, name: "WC", on: 0, c: "neutral" },
            { x: 68, y: 68, w: 28, h: 22, name: "Bedroom 2", on: 0, c: "cool" },
          ].map((r, i) => (
            <div key={i} style={{
              position: "absolute",
              left: `${r.x}%`, top: `${r.y}%`,
              width: `${r.w}%`, height: `${r.h}%`,
              borderRadius: 10,
              border: "1.5px solid var(--border)",
              background: r.on > 0 ? "var(--on-soft)" : "var(--card-2)",
              padding: 8,
              display: "flex",
              flexDirection: "column",
              justifyContent: "space-between"
            }}>
              <div style={{ fontSize: 11, fontWeight: 500, color: r.on > 0 ? "var(--on)" : "var(--text-mute)" }}>{r.name}</div>
              <div style={{ display: "flex", gap: 4 }}>
                {Array.from({ length: r.on }).map((_, j) => (
                  <div key={j} style={{ width: 8, height: 8, borderRadius: "50%", background: "var(--on)", boxShadow: "0 0 6px var(--on-glow)" }}/>
                ))}
                {r.on === 0 && <div style={{ fontFamily: "var(--font-mono)", fontSize: 9.5, color: "var(--text-dim)" }}>—</div>}
              </div>
            </div>
          ))}
        </div>
      </div>

      <SectionHead title="Tap a room"/>
      <div style={{ padding: "0 22px", color: "var(--text-mute)", fontSize: 13 }}>
        Drag devices on the plan to place them. <span style={{ color: "var(--cool)" }}>Edit mode</span> shows hidden devices.
      </div>

      <TabBar active="rooms"/>
    </div>
  );
}

Object.assign(window, { DevicesScreen, GroupsScreen, SensorsScreen, UsersScreen, SettingsScreen, FloorPlanScreen });
