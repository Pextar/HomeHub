/* HomeHub — desktop variants of secondary views. */

// shared layout with nav rail
function DesktopShell({ active, children, title, action }) {
  const nav = [
    { id: "home",      label: "Dashboard",  d: I.home },
    { id: "rooms",     label: "Rooms",      d: I.rooms },
    { id: "devices",   label: "Devices",    d: I.bulb },
    { id: "scenes",    label: "Scenes",     d: I.scenes },
    { id: "schedule",  label: "Schedules",  d: I.schedule },
    { id: "sensors",   label: "Sensors",    d: I.sensor },
    { id: "users",     label: "Users",      d: I.user },
    { id: "settings",  label: "Settings",   d: I.settings },
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
          <div key={n.id} className={`nav-item ${n.id === active ? "active" : ""}`}>
            <Icon d={n.d} size={17} stroke={1.7}/>
            <span>{n.label}</span>
          </div>
        ))}
        <div style={{ marginTop: "auto", padding: 12, background: "var(--card)", borderRadius: 12, border: "1px solid var(--hairline)", display: "flex", alignItems: "center", gap: 10 }}>
          <div style={{ width: 30, height: 30, borderRadius: "50%", background: "var(--card-3)", display: "grid", placeItems: "center", fontFamily: "var(--font-mono)", fontWeight: 600, fontSize: 12, color: "var(--on)" }}>M</div>
          <div style={{ minWidth: 0, flex: 1 }}>
            <div style={{ fontSize: 12.5, fontWeight: 500 }}>Mira</div>
            <div style={{ fontSize: 10.5, color: "var(--text-mute)" }}>Admin</div>
          </div>
        </div>
      </aside>
      <main style={{ flex: 1, padding: "28px 36px", overflow: "auto" }}>
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-end", marginBottom: 24 }}>
          <h1 style={{ fontSize: 28, fontWeight: 600, letterSpacing: "-0.03em" }}>{title}</h1>
          {action}
        </div>
        {children}
      </main>
    </div>
  );
}

// ── DESKTOP: SCHEDULES ──────────────────────────────────────
function DesktopSchedules() {
  const events = [
    { at: 6.5, d: "Coffee bar", a: "on", c: "var(--on)" },
    { at: 7, d: "Wake up", a: "run", c: "#ffd066" },
    { at: 8, d: "Coffee bar", a: "off", c: "var(--text-mute)" },
    { at: 17.5, d: "Porch", a: "sunset", c: "#d97a45" },
    { at: 19, d: "Evening", a: "run", c: "var(--on)" },
    { at: 23, d: "Hallway", a: "off", c: "var(--text-mute)" },
  ];
  const rows = [
    { name: "Porch lights",    sub: "Turn on",       days: "Every day", time: "At sunset (≈17:42)", on: true,  ico: I.sun },
    { name: "Coffee bar",      sub: "Turn on",       days: "Mon–Fri",   time: "06:30",              on: true,  ico: I.energy },
    { name: "Hallway",         sub: "Turn off",      days: "Every day", time: "23:00",              on: true,  ico: I.moon },
    { name: "Wake up scene",   sub: "Run scene",     days: "Mon–Fri",   time: "07:00",              on: true,  ico: I.scenes },
    { name: "Away · all off",  sub: "Run scene",     days: "Conditional", time: "—",                on: false, ico: I.power },
    { name: "Weekend night",   sub: "Turn off lights", days: "Sat, Sun", time: "01:00",            on: true,  ico: I.moon },
  ];
  return (
    <DesktopShell active="schedule" title="Schedules" action={
      <div style={{ display: "flex", gap: 8 }}>
        <button className="chip" style={{ padding: "9px 12px", fontSize: 13 }}><Icon d={I.plus} size={14} stroke={2}/> New schedule</button>
      </div>
    }>
      {/* timeline */}
      <div className="card" style={{ padding: 22, marginBottom: 22 }}>
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline", marginBottom: 18 }}>
          <div>
            <div style={{ color: "var(--text-mute)", fontSize: 11.5, fontFamily: "var(--font-mono)", letterSpacing: "0.1em", textTransform: "uppercase" }}>Today · Tuesday</div>
            <div style={{ fontSize: 17, fontWeight: 600, marginTop: 2 }}>5 events ahead</div>
          </div>
          <div style={{ display: "flex", gap: 8 }}>
            {["Day", "Week", "Month"].map((t, i) => (
              <button key={t} className={`chip ${i === 0 ? "active" : ""}`} style={{ padding: "6px 12px", fontSize: 12.5 }}>{t}</button>
            ))}
          </div>
        </div>
        <div style={{ position: "relative", height: 80 }}>
          <div style={{ position: "absolute", inset: 0, borderRadius: 14,
            background: "linear-gradient(90deg, #1a1d28 0%, #1a1d28 22%, #2a2618 28%, #3a2e1e 50%, #2a2618 72%, #1a1d28 78%, #1a1d28 100%)"
          }}/>
          <div style={{ position: "absolute", top: -8, bottom: -8, left: "40%", width: 2, background: "var(--text)", borderRadius: 1 }}>
            <div style={{ position: "absolute", top: -10, left: -3, width: 8, height: 8, borderRadius: "50%", background: "var(--text)" }}/>
            <div style={{ position: "absolute", top: -28, left: -16, fontFamily: "var(--font-mono)", fontSize: 10, color: "var(--text)", width: 36, textAlign: "center" }}>NOW</div>
          </div>
          {events.map((e, i) => (
            <div key={i} style={{ position: "absolute", left: `${(e.at/24)*100}%`, top: 20, transform: "translateX(-50%)", display: "flex", flexDirection: "column", alignItems: "center", gap: 6 }}>
              <div style={{ width: 12, height: 28, borderRadius: 4, background: e.c }}/>
              <div className="mono" style={{ fontSize: 9.5, color: "var(--text-mute)" }}>{`${Math.floor(e.at)}:${String(Math.round((e.at%1)*60)).padStart(2,"0")}`}</div>
            </div>
          ))}
        </div>
        <div style={{ display: "flex", justifyContent: "space-between", marginTop: 16, color: "var(--text-dim)", fontSize: 11, fontFamily: "var(--font-mono)" }}>
          {["00","03","06","09","12","15","18","21","24"].map(h => <span key={h}>{h}</span>)}
        </div>
      </div>

      {/* list */}
      <div className="card" style={{ padding: 0, overflow: "hidden" }}>
        {rows.map((s, i, arr) => (
          <React.Fragment key={s.name}>
            <div style={{ display: "flex", alignItems: "center", padding: "16px 20px", gap: 16 }}>
              <div style={{ width: 36, height: 36, borderRadius: 10, background: s.on ? "var(--on-soft)" : "var(--card-3)", display: "grid", placeItems: "center" }}>
                <Icon d={s.ico} size={16} stroke={1.7} style={{ color: s.on ? "var(--on)" : "var(--text-mute)" }}/>
              </div>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontWeight: 600, fontSize: 14.5, opacity: s.on ? 1 : 0.6 }}>{s.name}</div>
                <div style={{ color: "var(--text-mute)", fontSize: 12.5, marginTop: 2 }}>{s.sub} · {s.days}</div>
              </div>
              <div className="mono" style={{ fontSize: 13, color: s.on ? "var(--text)" : "var(--text-mute)", minWidth: 180, textAlign: "right" }}>{s.time}</div>
              <div className={`sw ${s.on ? "on" : ""}`}/>
            </div>
            {i < arr.length - 1 && <div className="sep" style={{ marginLeft: 72 }}/>}
          </React.Fragment>
        ))}
      </div>
    </DesktopShell>
  );
}

// ── DESKTOP: SENSORS ───────────────────────────────────────
function DesktopSensors() {
  const sensors = [
    { name: "Living room", kind: "Temperature", value: 21.4, unit: "°C", room: "Living", proto: "matter", spark: [20.8,20.9,21.1,21.0,21.2,21.4,21.4,21.4,21.5,21.4], alert: false },
    { name: "Living room", kind: "Humidity",    value: 42,   unit: "%",  room: "Living", proto: "matter", spark: [38,39,40,41,42,42,42,43,42,42], alert: false },
    { name: "Bedroom",     kind: "Temperature", value: 17.6, unit: "°C", room: "Bedroom", proto: "matter", spark: [19,18.6,18.4,18.0,17.8,17.6,17.6,17.6,17.5,17.6], alert: true },
    { name: "Outside",     kind: "Temperature", value: 12.0, unit: "°C", room: "Outside", proto: "mqtt", spark: [13,12.8,12.5,12.3,12.1,12.0,12.0,12.0,11.9,12.0], alert: false },
    { name: "Garage",      kind: "Power",       value: 184,  unit: "W",  room: "Garage", proto: "wifi", spark: [120,140,160,170,180,184,184,184,184,184], alert: false },
    { name: "Hallway",     kind: "Motion",      value: "Idle", room: "Hallway", proto: "matter", spark: null, alert: false, last: "12 min ago" },
  ];
  return (
    <DesktopShell active="sensors" title="Sensors" action={
      <button className="chip" style={{ padding: "9px 12px", fontSize: 13 }}><Icon d={I.plus} size={14} stroke={2}/> Pair sensor</button>
    }>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 14 }}>
        {sensors.map(s => (
          <div key={s.name+s.kind} className="card" style={{ padding: 18, gap: 12, borderColor: s.alert ? "var(--bad)" : "var(--hairline)" }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}>
              <div>
                <div style={{ color: "var(--text-mute)", fontSize: 11, fontFamily: "var(--font-mono)", letterSpacing: "0.08em", textTransform: "uppercase" }}>{s.kind}</div>
                <div style={{ fontSize: 15, fontWeight: 600, marginTop: 4 }}>{s.name}</div>
                <div style={{ color: "var(--text-mute)", fontSize: 12, marginTop: 2 }}>{s.room}</div>
              </div>
              {s.alert && <span style={{ fontFamily: "var(--font-mono)", fontSize: 10, color: "var(--bad)", fontWeight: 600, background: "rgba(224,138,122,0.14)", padding: "3px 7px", borderRadius: 6 }}>ALERT</span>}
            </div>
            <div style={{ display: "flex", alignItems: "baseline", gap: 6 }}>
              <span className="num-display" style={{ fontSize: 40, color: s.alert ? "var(--bad)" : "var(--text)" }}>{s.value}</span>
              {s.unit && <span style={{ color: "var(--text-mute)", fontSize: 14 }}>{s.unit}</span>}
            </div>
            {s.spark ? (
              <svg viewBox="0 0 100 24" style={{ width: "100%", height: 30 }} preserveAspectRatio="none">
                {(() => {
                  const min = Math.min(...s.spark), max = Math.max(...s.spark), range = max - min || 1;
                  const pts = s.spark.map((v, i) => `${(i / (s.spark.length - 1)) * 100},${24 - ((v - min) / range) * 22 - 1}`).join(" ");
                  const fillPts = pts + ` 100,24 0,24`;
                  return (
                    <>
                      <polygon points={fillPts} fill={s.alert ? "rgba(224,138,122,0.12)" : "rgba(132,172,196,0.1)"}/>
                      <polyline points={pts} fill="none" stroke={s.alert ? "var(--bad)" : "var(--cool)"} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/>
                    </>
                  );
                })()}
              </svg>
            ) : (
              <div style={{ color: "var(--text-dim)", fontSize: 12, marginTop: 2 }}>Last: {s.last}</div>
            )}
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <ProtocolBadge kind={s.proto}/>
              <span style={{ color: "var(--text-dim)", fontSize: 11.5 }}>updated 2s ago</span>
            </div>
          </div>
        ))}
      </div>
    </DesktopShell>
  );
}

// ── DESKTOP: USERS ─────────────────────────────────────────
function DesktopUsers() {
  const users = [
    { name: "Mira",   role: "Admin",   sub: "All devices",  initial: "M", color: "var(--on)",   sign: "Active now" },
    { name: "Theo",   role: "Limited", sub: "4 devices",    initial: "T", color: "var(--cool)", code: "5029", sign: "2 hours ago" },
    { name: "Alex",   role: "Limited", sub: "12 devices",   initial: "A", color: "#a96bd9",     code: "7142", sign: "Yesterday" },
    { name: "Cleaner", role: "Limited", sub: "2 devices",   initial: "C", color: "var(--good)", code: "3380", sign: "3 days ago" },
    { name: "Kids",   role: "Kid",     sub: "2 lamps",      initial: "K", color: "#d97a45",     code: "9011", sign: "Yesterday" },
  ];
  return (
    <DesktopShell active="users" title="Users" action={
      <button className="chip" style={{ padding: "9px 12px", fontSize: 13 }}><Icon d={I.plus} size={14} stroke={2}/> Invite user</button>
    }>
      <div className="card" style={{ padding: 0, overflow: "hidden" }}>
        <div style={{ display: "grid", gridTemplateColumns: "60px 1fr 1fr 120px 140px 100px 60px", padding: "12px 20px", color: "var(--text-mute)", fontSize: 11, fontFamily: "var(--font-mono)", letterSpacing: "0.1em", textTransform: "uppercase", background: "var(--card-2)", alignItems: "center" }}>
          <span></span><span>Name</span><span>Access</span><span>Login</span><span>Last seen</span><span>Role</span><span></span>
        </div>
        {users.map((u, i) => (
          <div key={u.name} style={{ display: "grid", gridTemplateColumns: "60px 1fr 1fr 120px 140px 100px 60px", padding: "16px 20px", alignItems: "center", borderTop: "1px solid var(--hairline)" }}>
            <div style={{ width: 36, height: 36, borderRadius: "50%", background: u.color, display: "grid", placeItems: "center", color: "#3a2400", fontWeight: 600, fontSize: 14, fontFamily: "var(--font-mono)" }}>{u.initial}</div>
            <div style={{ fontSize: 14, fontWeight: 600 }}>{u.name}</div>
            <div style={{ fontSize: 13, color: "var(--text-mute)" }}>{u.sub}</div>
            <div style={{ fontSize: 12.5, color: "var(--text-mute)", fontFamily: u.code ? "var(--font-mono)" : "inherit" }}>{u.code ? `Code ${u.code}` : "Password"}</div>
            <div style={{ fontSize: 12.5, color: u.sign === "Active now" ? "var(--good)" : "var(--text-mute)" }}>{u.sign}</div>
            <div>
              <span style={{ fontSize: 10.5, fontFamily: "var(--font-mono)", padding: "3px 8px", borderRadius: 6, background: u.role === "Admin" ? "var(--on-soft)" : "var(--card-3)", color: u.role === "Admin" ? "var(--on)" : "var(--text-mute)", letterSpacing: "0.04em" }}>{u.role.toUpperCase()}</span>
            </div>
            <Icon d={I.more} size={16} stroke={2} style={{ color: "var(--text-mute)", justifySelf: "end" }}/>
          </div>
        ))}
      </div>
    </DesktopShell>
  );
}

// ── DESKTOP: NOTIFICATIONS PANEL ───────────────────────────
function DesktopNotificationsPanel() {
  const groups = [
    { head: "Today", items: [
      { t: "18:42", title: "Evening scene activated", body: "8 devices · run from schedule", tone: "info", d: I.scenes, read: false },
      { t: "17:42", title: "Porch lights on",         body: "Sunset trigger",                tone: "info", d: I.sun,    read: false },
      { t: "14:08", title: "Hallway sensor lost signal", body: "Has not reported in 12 minutes", tone: "warn", d: I.bell, read: false },
      { t: "09:02", title: "Coffee bar turned off",   body: "Scheduled 08:00 · late by 2m",  tone: "info", d: I.energy, read: true },
    ]},
    { head: "Yesterday", items: [
      { t: "23:14", title: "Bedroom temperature low", body: "17.6° — threshold 18°",         tone: "warn", d: I.thermo, read: true },
      { t: "07:00", title: "Wake up scene",           body: "Gradual sunrise, 6 devices",    tone: "success", d: I.scenes, read: true },
    ]},
  ];
  const tone = (t) => t === "warn" ? "#e8b96b" : t === "error" ? "var(--bad)" : t === "success" ? "var(--good)" : "var(--cool)";

  return (
    <DesktopShell active="home" title="Dashboard">
      {/* mock content area on left + popover on right */}
      <div style={{ position: "relative", display: "grid", gridTemplateColumns: "1fr 380px", gap: 22 }}>
        <div style={{ display: "flex", flexDirection: "column", gap: 14, opacity: 0.35, pointerEvents: "none" }}>
          <div className="tile on" style={{ padding: 22, height: 160 }}>
            <div style={{ color: "var(--on)", fontSize: 11, fontFamily: "var(--font-mono)", letterSpacing: "0.12em", textTransform: "uppercase" }}>Whole home</div>
            <div style={{ marginTop: 10, display: "flex", alignItems: "baseline", gap: 10 }}>
              <span className="num-display" style={{ fontSize: 56 }}>7</span>
              <span style={{ color: "var(--text-mute)", fontSize: 14 }}>of 23 devices on</span>
            </div>
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 14 }}>
            {[1,2,3].map(i => <div key={i} className="card" style={{ height: 120 }}/>)}
          </div>
        </div>

        {/* notifications popover */}
        <div className="card" style={{ padding: 0, overflow: "hidden", position: "relative", alignSelf: "flex-start", boxShadow: "0 24px 64px rgba(0,0,0,0.5)" }}>
          {/* arrow pointing up toward bell icon would be here */}
          <div style={{ padding: "14px 18px", display: "flex", justifyContent: "space-between", alignItems: "center", borderBottom: "1px solid var(--hairline)" }}>
            <div style={{ fontSize: 15, fontWeight: 600 }}>Notifications</div>
            <div style={{ display: "flex", gap: 12 }}>
              <button style={{ color: "var(--text-mute)", fontSize: 12.5 }}>Mark all read</button>
              <button style={{ color: "var(--text-mute)" }}><Icon d={I.settings} size={14} stroke={1.7}/></button>
            </div>
          </div>

          <div style={{ maxHeight: 540, overflow: "auto" }}>
            {groups.map(g => (
              <React.Fragment key={g.head}>
                <div style={{ padding: "12px 18px 6px", color: "var(--text-mute)", fontSize: 10.5, fontFamily: "var(--font-mono)", letterSpacing: "0.1em", textTransform: "uppercase", background: "var(--card-2)" }}>{g.head}</div>
                {g.items.map((it, i) => (
                  <div key={i} style={{ padding: "12px 18px", display: "flex", gap: 12, alignItems: "flex-start", opacity: it.read ? 0.65 : 1, borderTop: i === 0 ? "0" : "1px solid var(--hairline)" }}>
                    <div style={{ width: 30, height: 30, borderRadius: 8, background: it.read ? "var(--card-3)" : "var(--on-soft)", display: "grid", placeItems: "center", flexShrink: 0 }}>
                      <Icon d={it.d} size={14} stroke={1.7} style={{ color: it.read ? "var(--text-mute)" : tone(it.tone) }}/>
                    </div>
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline" }}>
                        <div style={{ fontSize: 13, fontWeight: 600, color: it.read ? "var(--text-mute)" : "var(--text)" }}>{it.title}</div>
                        <div className="mono" style={{ fontSize: 10.5, color: "var(--text-dim)", flexShrink: 0 }}>{it.t}</div>
                      </div>
                      <div style={{ color: "var(--text-mute)", fontSize: 11.5, marginTop: 2, lineHeight: 1.35 }}>{it.body}</div>
                    </div>
                    {!it.read && <div style={{ width: 7, height: 7, borderRadius: "50%", background: "var(--on)", marginTop: 8, flexShrink: 0 }}/>}
                  </div>
                ))}
              </React.Fragment>
            ))}
          </div>

          <div style={{ padding: "10px 18px", borderTop: "1px solid var(--hairline)", textAlign: "center" }}>
            <button style={{ color: "var(--on)", fontSize: 12.5, fontWeight: 500 }}>Notification settings</button>
          </div>
        </div>
      </div>
    </DesktopShell>
  );
}

Object.assign(window, {
  DesktopShell, DesktopSchedules, DesktopSensors, DesktopUsers, DesktopNotificationsPanel,
});
