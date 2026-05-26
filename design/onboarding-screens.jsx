/* HomeHub — onboarding + notification screens. */

// ── shared: phone-sized empty screen with brand header ────────
function PhoneShell({ children, bg, footer }) {
  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "hidden", background: bg || "var(--bg)", display: "flex", flexDirection: "column" }}>
      <div style={{ height: 54 }}/>
      {children}
      {footer}
    </div>
  );
}

// brand mark — small geometric "switch" tile
const Brand = ({ size = 36 }) => (
  <div style={{ width: size, height: size, borderRadius: size * 0.28, background: "var(--on)", display: "grid", placeItems: "center" }}>
    <div style={{ width: size * 0.42, height: size * 0.42, borderRadius: size * 0.1, background: "var(--bg)" }}/>
  </div>
);

// ── 1. LOGIN (code mode) ────────────────────────────────────
function LoginCodeScreen() {
  const code = "5 0 2 9";
  return (
    <PhoneShell bg="linear-gradient(180deg, #2a2218 0%, var(--bg) 40%)">
      <div style={{ padding: "40px 22px 0", textAlign: "center" }}>
        <div style={{ display: "inline-block" }}><Brand size={56}/></div>
        <h1 style={{ fontSize: 28, fontWeight: 600, marginTop: 22, letterSpacing: "-0.03em" }}>HomeHub</h1>
        <p style={{ color: "var(--text-mute)", fontSize: 14, marginTop: 6 }}>Enter your login code</p>
      </div>

      <div style={{ padding: "44px 22px 0" }}>
        {/* 4-digit code input */}
        <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 10, marginBottom: 22 }}>
          {code.split(" ").map((d, i) => (
            <div key={i} className="card" style={{ aspectRatio: "1", display: "grid", placeItems: "center", borderColor: i === 3 ? "var(--on)" : "var(--hairline)", borderWidth: i === 3 ? 2 : 1, background: i === 3 ? "var(--on-soft)" : "var(--card)" }}>
              <span className="num-display" style={{ fontSize: 36, color: "var(--text)" }}>{d}</span>
            </div>
          ))}
        </div>

        <button style={{ width: "100%", padding: "16px", borderRadius: 22, background: "var(--on)", color: "#3a2400", fontWeight: 600, fontSize: 15 }}>
          Sign in
        </button>
        <button style={{ width: "100%", padding: "16px", color: "var(--text-mute)", fontSize: 13, marginTop: 4 }}>
          Sign in as admin
        </button>
      </div>

      {/* numeric keypad */}
      <div style={{ marginTop: "auto", padding: "0 22px 22px" }}>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 8 }}>
          {[1,2,3,4,5,6,7,8,9,"",0,"⌫"].map((k, i) => (
            <button key={i} style={{ height: 56, borderRadius: 16, background: k === "" ? "transparent" : "var(--card)", border: k === "" ? "0" : "1px solid var(--hairline)", color: "var(--text)", fontSize: 22, fontWeight: 500 }}>
              {k}
            </button>
          ))}
        </div>
      </div>
    </PhoneShell>
  );
}

// ── 2. LOGIN (admin) ────────────────────────────────────────
function LoginAdminScreen() {
  return (
    <PhoneShell bg="linear-gradient(180deg, #2a2218 0%, var(--bg) 40%)">
      <div style={{ padding: "40px 22px 0", textAlign: "center" }}>
        <div style={{ display: "inline-block" }}><Brand size={56}/></div>
        <h1 style={{ fontSize: 28, fontWeight: 600, marginTop: 22, letterSpacing: "-0.03em" }}>HomeHub</h1>
        <p style={{ color: "var(--text-mute)", fontSize: 14, marginTop: 6 }}>Sign in as admin</p>
      </div>

      <div style={{ padding: "40px 22px 0", display: "flex", flexDirection: "column", gap: 14 }}>
        <Field label="Username">
          <input defaultValue="mira" style={inputStyle}/>
        </Field>
        <Field label="Password">
          <input type="password" defaultValue="••••••••••" style={inputStyle}/>
        </Field>

        <button style={{ width: "100%", padding: "16px", borderRadius: 22, background: "var(--on)", color: "#3a2400", fontWeight: 600, fontSize: 15, marginTop: 8 }}>
          Sign in
        </button>
        <button style={{ width: "100%", padding: "12px", color: "var(--text-mute)", fontSize: 13 }}>
          Use a login code instead
        </button>
      </div>
    </PhoneShell>
  );
}

const inputStyle = {
  width: "100%",
  padding: "14px 16px",
  borderRadius: 14,
  background: "var(--card)",
  border: "1px solid var(--hairline)",
  color: "var(--text)",
  fontSize: 15,
  fontFamily: "inherit",
  outline: "none",
};

function Field({ label, children, help }) {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
      <label style={{ fontSize: 12.5, color: "var(--text-mute)", fontWeight: 500 }}>{label}</label>
      {children}
      {help && <span style={{ fontSize: 11.5, color: "var(--text-dim)" }}>{help}</span>}
    </div>
  );
}

// ── 3. INVITE: SET PASSWORD ─────────────────────────────────
function InviteSetPasswordScreen() {
  return (
    <PhoneShell bg="linear-gradient(180deg, #2a2218 0%, var(--bg) 40%)">
      <div style={{ padding: "40px 22px 0", textAlign: "center" }}>
        <div style={{ display: "inline-block" }}><Brand size={56}/></div>
        <h1 style={{ fontSize: 26, fontWeight: 600, marginTop: 22, letterSpacing: "-0.03em" }}>Welcome, Theo</h1>
        <p style={{ color: "var(--text-mute)", fontSize: 14, marginTop: 6, maxWidth: 280, marginLeft: "auto", marginRight: "auto" }}>
          Set a password to get started on HomeHub.
        </p>
      </div>

      <div style={{ padding: "32px 22px 0", display: "flex", flexDirection: "column", gap: 14 }}>
        <Field label="Password" help="At least 8 characters">
          <div style={{ position: "relative" }}>
            <input type="password" defaultValue="••••••••••" style={inputStyle}/>
            <button style={{ position: "absolute", right: 10, top: "50%", transform: "translateY(-50%)", color: "var(--text-mute)", fontSize: 12, padding: "4px 8px", borderRadius: 8, background: "var(--card-3)" }}>Show</button>
          </div>
        </Field>
        <Field label="Confirm password">
          <input type="password" defaultValue="••••••••••" style={inputStyle}/>
        </Field>

        {/* strength meter */}
        <div style={{ marginTop: 4 }}>
          <div style={{ display: "flex", gap: 4 }}>
            {[1,2,3,4].map(i => (
              <div key={i} style={{ flex: 1, height: 4, borderRadius: 2, background: i <= 3 ? "var(--on)" : "var(--card-3)" }}/>
            ))}
          </div>
          <div style={{ display: "flex", justifyContent: "space-between", marginTop: 6 }}>
            <span style={{ fontSize: 11.5, color: "var(--text-mute)" }}>Strength</span>
            <span style={{ fontSize: 11.5, color: "var(--on)", fontWeight: 500 }}>Strong</span>
          </div>
        </div>

        <button style={{ width: "100%", padding: "16px", borderRadius: 22, background: "var(--on)", color: "#3a2400", fontWeight: 600, fontSize: 15, marginTop: 12 }}>
          Set password & sign in
        </button>
      </div>

      <div style={{ marginTop: "auto", padding: "0 22px 30px", color: "var(--text-dim)", fontSize: 11.5, textAlign: "center" }}>
        Invited by Mira · expires in 6 days
      </div>
    </PhoneShell>
  );
}

// ── 4. PUSH PERMISSION PROMPT ───────────────────────────────
function PushPermissionScreen() {
  return (
    <PhoneShell>
      <div style={{ padding: `0 22px`, display: "flex", flexDirection: "column", gap: 14 }}>
        <button style={{ alignSelf: "flex-start", color: "var(--text-mute)", fontSize: 14, padding: "8px 0" }}>
          ← Settings
        </button>
        <h1 style={{ fontSize: 26, fontWeight: 600, letterSpacing: "-0.03em" }}>Notifications</h1>

        {/* hero permission card */}
        <div className="tile on" style={{ padding: 22, gap: 14, marginTop: 8 }}>
          <div style={{ width: 48, height: 48, borderRadius: 14, background: "var(--on)", display: "grid", placeItems: "center" }}>
            <Icon d={I.bell} size={22} stroke={1.9} style={{ color: "#3a2400" }}/>
          </div>
          <div>
            <div style={{ fontSize: 18, fontWeight: 600 }}>Get notified when things happen</div>
            <div style={{ color: "var(--text-mute)", fontSize: 13, marginTop: 6, lineHeight: 1.45 }}>
              Sensor alerts, schedule failures, and other home events — pushed to this device even when HomeHub isn't open.
            </div>
          </div>
          <button style={{ width: "100%", padding: "14px", borderRadius: 16, background: "var(--on)", color: "#3a2400", fontWeight: 600, fontSize: 14, marginTop: 6 }}>
            Turn on notifications
          </button>
        </div>

        <div style={{ marginTop: 6 }}>
          <div style={{ color: "var(--text-mute)", fontSize: 11.5, fontFamily: "var(--font-mono)", letterSpacing: "0.08em", textTransform: "uppercase", marginBottom: 10, padding: "0 4px" }}>You'll be alerted about</div>
          <div className="card" style={{ padding: 0, overflow: "hidden" }}>
            {[
              { d: I.thermo,    l: "Temperature out of range", s: "Bedroom under 18°, Living over 26°" },
              { d: I.motion,    l: "Motion when away", s: "Any motion sensor while Away is active" },
              { d: I.power,     l: "Schedule failures", s: "When a device doesn't respond" },
              { d: I.energy,    l: "Unusual energy use", s: "Above 200 W after midnight" },
            ].map((row, i, a) => (
              <React.Fragment key={row.l}>
                <div style={{ display: "flex", alignItems: "flex-start", padding: "14px 16px", gap: 12 }}>
                  <div style={{ width: 30, height: 30, borderRadius: 8, background: "var(--card-3)", display: "grid", placeItems: "center", flexShrink: 0, marginTop: 1 }}>
                    <Icon d={row.d} size={15} stroke={1.7} style={{ color: "var(--on)" }}/>
                  </div>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ fontSize: 14, fontWeight: 500 }}>{row.l}</div>
                    <div style={{ color: "var(--text-mute)", fontSize: 12, marginTop: 2 }}>{row.s}</div>
                  </div>
                </div>
                {i < a.length - 1 && <div className="sep" style={{ marginLeft: 60 }}/>}
              </React.Fragment>
            ))}
          </div>
          <div style={{ color: "var(--text-dim)", fontSize: 11.5, padding: "10px 4px 0", textAlign: "center" }}>
            You can customize alerts per-sensor in Settings later.
          </div>
        </div>
      </div>
    </PhoneShell>
  );
}

// ── 5. NOTIFICATIONS INBOX ──────────────────────────────────
function NotificationsInboxScreen() {
  const groups = [
    {
      head: "Today",
      items: [
        { t: "18:42", title: "Evening scene activated", body: "8 devices · run from schedule", tone: "info", d: I.scenes, read: false },
        { t: "17:42", title: "Porch lights on", body: "Sunset trigger", tone: "info", d: I.sun, read: false },
        { t: "14:08", title: "Hallway sensor lost signal", body: "Has not reported in 12 minutes", tone: "warn", d: I.bell, read: false },
        { t: "09:02", title: "Coffee bar turned off", body: "Scheduled 08:00 · ran late by 2m", tone: "info", d: I.energy, read: true },
      ],
    },
    {
      head: "Yesterday",
      items: [
        { t: "23:14", title: "Bedroom temperature low", body: "17.6° — threshold 18°", tone: "warn", d: I.thermo, read: true },
        { t: "07:00", title: "Wake up scene", body: "Gradual sunrise, 6 devices", tone: "success", d: I.scenes, read: true },
      ],
    },
  ];

  const toneColor = (t) =>
    t === "warn"    ? "#e8b96b" :
    t === "error"   ? "var(--bad)" :
    t === "success" ? "var(--good)" : "var(--cool)";

  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "hidden", paddingBottom: 90 }}>
      <div style={{ height: 54 }}/>

      <div style={{ padding: `4px 22px 0`, display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <button style={{ color: "var(--text-mute)", fontSize: 14 }}>← Home</button>
        <button style={{ color: "var(--text-mute)", fontSize: 13 }}>Mark all read</button>
      </div>

      <div style={{ padding: `8px 22px 0` }}>
        <h1 style={{ fontSize: 28, fontWeight: 600, letterSpacing: "-0.03em" }}>Notifications</h1>
        <div style={{ color: "var(--text-mute)", fontSize: 13, marginTop: 2 }}>
          <span className="mono" style={{ color: "var(--on)" }}>3</span> unread · 6 total today
        </div>
      </div>

      {/* filter chips */}
      <div className="h-scroll" style={{ marginTop: 16 }}>
        {["All", "Unread", "Alerts", "Schedules", "Devices"].map((c, i) => (
          <button key={c} className={`chip ${i === 0 ? "active" : ""}`}>{c}</button>
        ))}
      </div>

      {groups.map(g => (
        <React.Fragment key={g.head}>
          <div style={{ padding: "22px 22px 8px", color: "var(--text-mute)", fontSize: 11.5, fontFamily: "var(--font-mono)", letterSpacing: "0.1em", textTransform: "uppercase" }}>
            {g.head}
          </div>
          <div style={{ padding: "0 22px", display: "flex", flexDirection: "column", gap: 8 }}>
            {g.items.map((it, i) => (
              <div key={i} className="card" style={{ padding: 14, flexDirection: "row", display: "flex", gap: 12, alignItems: "flex-start", opacity: it.read ? 0.7 : 1 }}>
                <div style={{ width: 36, height: 36, borderRadius: 10, background: it.read ? "var(--card-3)" : "var(--on-soft)", display: "grid", placeItems: "center", flexShrink: 0, position: "relative" }}>
                  <Icon d={it.d} size={17} stroke={1.7} style={{ color: it.read ? "var(--text-mute)" : toneColor(it.tone) }}/>
                </div>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline", gap: 8 }}>
                    <div style={{ fontWeight: 600, fontSize: 14, color: it.read ? "var(--text-mute)" : "var(--text)" }}>{it.title}</div>
                    <div className="mono" style={{ fontSize: 11.5, color: "var(--text-dim)", flexShrink: 0 }}>{it.t}</div>
                  </div>
                  <div style={{ color: "var(--text-mute)", fontSize: 12.5, marginTop: 2, lineHeight: 1.4 }}>{it.body}</div>
                </div>
                {!it.read && <div style={{ width: 7, height: 7, borderRadius: "50%", background: "var(--on)", marginTop: 14, flexShrink: 0 }}/>}
              </div>
            ))}
          </div>
        </React.Fragment>
      ))}

      <TabBar active="home"/>
    </div>
  );
}

// ── 6. KID HOME ─────────────────────────────────────────────
function KidHomeScreen() {
  const lamps = [
    { name: "Nightlight", emoji: "🌙", on: true,  c: "#a96bd9" },
    { name: "Stars",      emoji: "✨", on: false, c: "var(--on)" },
    { name: "Reading",    emoji: "📚", on: false, c: "#d97a45" },
    { name: "Desk",       emoji: "✏️", on: true,  c: "#7aa4d9" },
  ];
  // (Kid mode is the one place where playful emoji is allowed by spec — see KidHome.svelte.)
  return (
    <PhoneShell bg="linear-gradient(180deg, #3a2f1f 0%, #1f1d17 60%)">
      <div style={{ padding: "20px 22px 0", display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <div>
          <div style={{ color: "var(--text-mute)", fontSize: 13 }}>Hi, Theo</div>
          <h1 style={{ fontSize: 30, fontWeight: 700, marginTop: 4 }}>My lamps</h1>
        </div>
        <button style={{ width: 40, height: 40, borderRadius: 12, background: "var(--card)", display: "grid", placeItems: "center", border: "1px solid var(--hairline)" }}>
          <Icon d={I.power} size={18} stroke={1.7} style={{ color: "var(--text-mute)" }}/>
        </button>
      </div>

      <div style={{ padding: "24px 22px 0", display: "grid", gridTemplateColumns: "1fr 1fr", gap: 14 }}>
        {lamps.map(l => (
          <div key={l.name} className={`tile ${l.on ? "on" : ""}`} style={{ padding: 18, gap: 14, height: 180, alignItems: "center", justifyContent: "center", textAlign: "center", borderRadius: 24 }}>
            <div style={{ fontSize: 56, lineHeight: 1, opacity: l.on ? 1 : 0.45, filter: l.on ? `drop-shadow(0 0 12px ${l.c})` : "none" }}>{l.emoji}</div>
            <div>
              <div style={{ fontSize: 17, fontWeight: 600 }}>{l.name}</div>
              <div style={{ color: l.on ? "var(--on)" : "var(--text-mute)", fontSize: 13, marginTop: 4, fontWeight: 500 }}>{l.on ? "ON" : "OFF"}</div>
            </div>
          </div>
        ))}
      </div>

      <div style={{ marginTop: "auto", padding: "0 22px 30px" }}>
        <button style={{ width: "100%", padding: "16px", borderRadius: 22, background: "var(--card)", border: "1px solid var(--hairline)", color: "var(--text-mute)", fontSize: 14, fontWeight: 500 }}>
          Turn everything off
        </button>
      </div>
    </PhoneShell>
  );
}

Object.assign(window, {
  LoginCodeScreen, LoginAdminScreen, InviteSetPasswordScreen,
  PushPermissionScreen, NotificationsInboxScreen, KidHomeScreen,
  PhoneShell, Brand, Field, inputStyle,
});
