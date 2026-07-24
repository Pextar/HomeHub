/* HomeHub — desktop dashboard. Designed for ~1280×800 within a browser
   window frame. Sidebar nav + main content grid. */

function DesktopDashboard() {
  const devices = [
    { name: "Floor lamp",    room: "Living room",  on: true,  dim: 62,  protocol: "matter", power: "18 W" },
    { name: "TV strip",      room: "Living room",  on: true,  dim: 28,  protocol: "matter", power: "6 W" },
    { name: "Sofa lamp",     room: "Living room",  on: false, protocol: "rf" },
    { name: "Kitchen isle",  room: "Kitchen",      on: true,  dim: 100, protocol: "wifi",   power: "44 W" },
    { name: "Under cabinet", room: "Kitchen",      on: true,  dim: 80,  protocol: "wifi",   power: "12 W" },
    { name: "Coffee bar",    room: "Kitchen",      on: false, protocol: "rf" },
    { name: "Porch",         room: "Outside",      on: true,  protocol: "rf",  power: "9 W" },
    { name: "Garage",        room: "Outside",      on: false, protocol: "rf" },
    { name: "Nightstand L",  room: "Bedroom",      on: false, protocol: "matter" },
    { name: "Nightstand R",  room: "Bedroom",      on: false, protocol: "matter" },
    { name: "Bedroom main",  room: "Bedroom",      on: false, protocol: "matter" },
    { name: "Hallway",       room: "Hallway",      on: true,  dim: 30,  protocol: "rf",     power: "8 W" },
  ];

  const nav = [
    { id: "home",      label: "Dashboard",  d: I.home,     active: true },
    { id: "rooms",     label: "Rooms",      d: I.rooms },
    { id: "music",     label: "Music",      d: I.music },
    { id: "devices",   label: "Devices",    d: I.bulb },
    { id: "scenes",    label: "Scenes",     d: I.scenes },
    { id: "schedule",  label: "Schedules",  d: I.schedule },
    { id: "sensors",   label: "Sensors",    d: I.sensor },
    { id: "users",     label: "Users",      d: I.user },
    { id: "settings",  label: "Settings",   d: I.settings },
  ];

  return (
    <div className="hh" style={{ height: "100%", display: "flex", overflow: "hidden" }}>

      {/* ── nav rail ── */}
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
          <div key={n.id} className={`nav-item ${n.active ? "active" : ""}`}>
            <Icon d={n.d} size={17} stroke={1.7}/>
            <span>{n.label}</span>
            {n.id === "schedule" && <span style={{ marginLeft: "auto", fontFamily: "var(--font-mono)", fontSize: 10, color: "var(--text-mute)", background: "var(--card-3)", padding: "2px 6px", borderRadius: 6 }}>5</span>}
          </div>
        ))}

        <div style={{ marginTop: "auto", padding: 12, background: "var(--card)", borderRadius: 12, border: "1px solid var(--hairline)", display: "flex", alignItems: "center", gap: 10 }}>
          <div style={{ width: 30, height: 30, borderRadius: "50%", background: "var(--card-3)", display: "grid", placeItems: "center", fontFamily: "var(--font-mono)", fontWeight: 600, fontSize: 12, color: "var(--on)" }}>M</div>
          <div style={{ minWidth: 0, flex: 1 }}>
            <div style={{ fontSize: 12.5, fontWeight: 500, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>Mira</div>
            <div style={{ fontSize: 10.5, color: "var(--text-mute)" }}>Admin</div>
          </div>
          <Icon d={I.chevR} size={14} stroke={1.7} style={{ color: "var(--text-dim)" }}/>
        </div>
      </aside>

      {/* ── main ── */}
      <main style={{ flex: 1, padding: "28px 36px", overflow: "auto" }}>
        {/* topbar */}
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", marginBottom: 28 }}>
          <div>
            <div style={{ color: "var(--text-mute)", fontSize: 12.5, fontWeight: 500 }}>Tuesday · May 23</div>
            <h1 style={{ fontSize: 30, fontWeight: 600, marginTop: 4, letterSpacing: "-0.03em" }}>
              Good evening, Mira
            </h1>
          </div>
          <div style={{ display: "flex", gap: 8 }}>
            <button className="chip" style={{ padding: "9px 12px", fontSize: 13 }}>
              <Icon d={I.search} size={14} stroke={1.7}/>
              Search devices, scenes…
              <span style={{ marginLeft: 28, color: "var(--text-dim)", fontFamily: "var(--font-mono)", fontSize: 11 }}>⌘K</span>
            </button>
            <button className="chip" style={{ padding: "9px 12px", fontSize: 13 }}>
              <Icon d={I.plus} size={14} stroke={2}/> Add device
            </button>
          </div>
        </div>

        {/* stat row */}
        <div style={{ display: "grid", gridTemplateColumns: "1.6fr 1fr 1fr 1fr", gap: 14, marginBottom: 22 }}>
          {/* hero */}
          <div className="tile on" style={{ padding: 22, gap: 18 }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}>
              <div>
                <div style={{ color: "var(--on)", fontSize: 11, fontFamily: "var(--font-mono)", letterSpacing: "0.12em", textTransform: "uppercase" }}>Whole home</div>
                <div style={{ marginTop: 10, display: "flex", alignItems: "baseline", gap: 10 }}>
                  <span className="num-display" style={{ fontSize: 64 }}>7</span>
                  <span style={{ color: "var(--text-mute)", fontSize: 15 }}>of 23 devices on</span>
                </div>
              </div>
              <div className="sw-big on"/>
            </div>
            <div style={{ display: "flex", gap: 18, color: "var(--text-mute)", fontSize: 12.5 }}>
              <span><Icon d={I.energy} size={13} stroke={1.7} style={{ color: "var(--on)", verticalAlign: "-2px", marginRight: 4 }}/><span className="mono" style={{ color: "var(--text)" }}>184 W</span> now</span>
              <span>· <span className="mono" style={{ color: "var(--text)" }}>3.2 kWh</span> today</span>
              <span>· next event in <span className="mono" style={{ color: "var(--text)" }}>4h 12m</span></span>
            </div>
          </div>

          {[
            { l: "Active scene", v: "Evening", sub: "Since 18:42", c: "var(--on)" },
            { l: "Indoor temp",  v: "21°",     sub: "Living room", c: "var(--cool)" },
            { l: "Sunset",       v: "17:42",   sub: "in 4h 12m",   c: "#d97a45" },
          ].map(s => (
            <div key={s.l} className="tile" style={{ padding: 18, gap: 8, justifyContent: "space-between" }}>
              <div style={{ color: "var(--text-mute)", fontSize: 11.5, fontFamily: "var(--font-mono)", letterSpacing: "0.08em", textTransform: "uppercase" }}>{s.l}</div>
              <div style={{ fontSize: 32, fontWeight: 600, color: s.c, letterSpacing: "-0.02em" }}>{s.v}</div>
              <div style={{ color: "var(--text-mute)", fontSize: 12 }}>{s.sub}</div>
            </div>
          ))}
        </div>

        {/* scenes row */}
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline", marginBottom: 12 }}>
          <h2 style={{ fontSize: 17, fontWeight: 600 }}>Scenes</h2>
          <button style={{ color: "var(--text-mute)", fontSize: 13 }}>Manage →</button>
        </div>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(6, 1fr)", gap: 10, marginBottom: 26 }}>
          {[
            { n: "Evening",   c: "var(--on)",   active: true, sub: "8 devices" },
            { n: "Goodnight", c: "var(--cool)", sub: "All off" },
            { n: "Movie",     c: "#a96bd9",     sub: "Living rm" },
            { n: "Read",      c: "#d97a45",     sub: "Bedroom" },
            { n: "Wake up",   c: "#ffd066",     sub: "07:00 daily" },
            { n: "Away",      c: "var(--text-mute)", sub: "23 devices" },
          ].map(s => (
            <div key={s.n} className="tile" style={{ padding: 14, gap: 10, borderColor: s.active ? "var(--on)" : "var(--hairline)" }}>
              <div style={{ width: 22, height: 22, borderRadius: "50%", background: s.c, opacity: s.active ? 1 : 0.45 }}/>
              <div>
                <div style={{ fontWeight: 600, fontSize: 14 }}>{s.n}</div>
                <div style={{ color: "var(--text-mute)", fontSize: 11.5, marginTop: 2 }}>{s.sub}</div>
              </div>
            </div>
          ))}
        </div>

        {/* devices section */}
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline", marginBottom: 12 }}>
          <h2 style={{ fontSize: 17, fontWeight: 600 }}>Devices</h2>
          <div style={{ display: "flex", gap: 4 }}>
            {["All", "On", "Living room", "Kitchen", "Bedroom", "Outside"].map((c, i) => (
              <button key={c} className={`chip ${i === 0 ? "active" : ""}`} style={{ padding: "6px 12px", fontSize: 12.5 }}>{c}</button>
            ))}
          </div>
        </div>

        <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 12 }}>
          {devices.map(d => (
            <div key={d.name} className={`tile ${d.on ? "on" : ""}`} style={{ padding: 16, gap: 12 }}>
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}>
                <div className="tile-bulb">
                  <Icon d={I.bulb} size={18} stroke={1.7} style={{ color: d.on ? "#3a2400" : "var(--text-mute)" }}/>
                </div>
                <div className={`sw ${d.on ? "on" : ""}`}/>
              </div>
              <div>
                <div style={{ fontWeight: 600, fontSize: 15 }}>{d.name}</div>
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginTop: 4 }}>
                  <div style={{ color: "var(--text-mute)", fontSize: 12 }}>
                    {d.room} · {d.on ? (d.dim != null ? `${d.dim}%` : "On") : "Off"}
                  </div>
                  <ProtocolBadge kind={d.protocol}/>
                </div>
                {d.on && d.dim != null && (
                  <div className="rail" style={{ marginTop: 8 }}><i style={{ width: `${d.dim}%` }}/></div>
                )}
                {d.on && d.power && (
                  <div style={{ color: "var(--text-dim)", fontSize: 11, fontFamily: "var(--font-mono)", marginTop: 8 }}>{d.power}</div>
                )}
              </div>
            </div>
          ))}
        </div>
      </main>
    </div>
  );
}

Object.assign(window, { DesktopDashboard });
