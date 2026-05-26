/* HomeHub — two integrated pages adopted from the out-of-the-box exploration.
   Spatial blueprint lives in the Rooms tab as "Floor plan".
   Console lives behind Settings → System → Console.
   Both wear the app's standard chrome (status bar pad, header, tab bar). */

// shared header — back chip + centered title + optional action chip
function PageHeader({ title, subtitle, action }) {
  return (
    <div style={{ padding: "4px 22px", display: "flex", justifyContent: "space-between", alignItems: "center", gap: 12 }}>
      <button className="chip" style={{ width: 36, height: 36, padding: 0, justifyContent: "center", flexShrink: 0 }}>
        <Icon d={I.back} size={16} stroke={2}/>
      </button>
      <div style={{ textAlign: "center", flex: 1, minWidth: 0 }}>
        <div style={{ fontSize: 15, fontWeight: 600 }}>{title}</div>
        {subtitle && <div style={{ color: "var(--text-mute)", fontSize: 11.5 }}>{subtitle}</div>}
      </div>
      {action ?? <div style={{ width: 36, height: 36, flexShrink: 0 }}/>}
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// SPATIAL BLUEPRINT — adopted as the "Floor plan" page
// (replaces the existing FloorPlanScreen)
// ─────────────────────────────────────────────────────────────

function SpatialBlueprintScreen() {
  const lights = [
    { id: "L1", room: "Living", name: "Floor lamp", x:  80, y: 110, on: true,  dim: 62, hue: "#f5bd6e" },
    { id: "L2", room: "Living", name: "TV strip",   x: 160, y:  70, on: true,  dim: 28, hue: "#f5bd6e" },
    { id: "L3", room: "Living", name: "Reading",    x:  60, y: 170, on: false, dim:  0, hue: "#f5bd6e" },
    { id: "K1", room: "Kitch.", name: "Island",     x: 250, y:  90, on: true,  dim:100, hue: "#ffe9c4" },
    { id: "K2", room: "Kitch.", name: "Coffee bar", x: 300, y: 150, on: false, dim:  0, hue: "#f5bd6e" },
    { id: "H1", room: "Hall",   name: "Hallway",    x: 180, y: 230, on: true,  dim: 40, hue: "#f5bd6e" },
    { id: "B1", room: "Bed",    name: "Nightstand", x:  80, y: 330, on: false, dim:  0, hue: "#c4a4e0" },
    { id: "B2", room: "Bed",    name: "Ceiling",    x: 130, y: 360, on: false, dim:  0, hue: "#f5bd6e" },
    { id: "T1", room: "Bath",   name: "Mirror",     x: 270, y: 330, on: false, dim:  0, hue: "#ffffff" },
    { id: "P1", room: "Porch",  name: "Porch",      x: 320, y: 420, on: true,  dim: 80, hue: "#d97a45" },
  ];

  const wallColor = "rgba(245,189,110,0.35)";
  const wallSubtle = "rgba(245,189,110,0.18)";

  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "hidden", background: "#0d0c08", paddingBottom: 90 }}>
      <div style={{ height: 54 }}/>

      <PageHeader
        title="Floor plan"
        subtitle="04 on · 184 W · 21°C"
        action={
          <button className="chip" style={{ width: 36, height: 36, padding: 0, justifyContent: "center", flexShrink: 0 }}>
            <Icon d={I.settings} size={16} stroke={1.7}/>
          </button>
        }
      />

      {/* tiny technical strip below header */}
      <div style={{ padding: "10px 22px 0", display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <span className="mono" style={{ color: "var(--on)", fontSize: 10, letterSpacing: "0.18em" }}>PLAN ⌁ N→</span>
        <span className="mono" style={{ color: "var(--text-dim)", fontSize: 10, letterSpacing: "0.08em" }}>4.2 × 7.1 m</span>
      </div>

      {/* blueprint */}
      <div style={{ padding: "10px 16px 0" }}>
        <svg viewBox="0 0 360 460" width="100%" style={{ display: "block" }}>
          <defs>
            <radialGradient id="glow-warm-2" cx="50%" cy="50%" r="50%">
              <stop offset="0%"  stopColor="#f5bd6e" stopOpacity="0.9"/>
              <stop offset="50%" stopColor="#f5bd6e" stopOpacity="0.25"/>
              <stop offset="100%" stopColor="#f5bd6e" stopOpacity="0"/>
            </radialGradient>
            <radialGradient id="glow-cool-2" cx="50%" cy="50%" r="50%">
              <stop offset="0%"  stopColor="#ffe9c4" stopOpacity="0.7"/>
              <stop offset="100%" stopColor="#ffe9c4" stopOpacity="0"/>
            </radialGradient>
            <pattern id="grid-2" width="20" height="20" patternUnits="userSpaceOnUse">
              <path d="M 20 0 L 0 0 0 20" fill="none" stroke="rgba(245,189,110,0.05)" strokeWidth="0.5"/>
            </pattern>
          </defs>

          <rect x="0" y="0" width="360" height="460" fill="url(#grid-2)"/>

          {lights.filter(l => l.on).map(l => (
            <circle key={"g"+l.id} cx={l.x} cy={l.y} r={l.dim * 0.7 + 25}
              fill={l.hue === "#ffe9c4" ? "url(#glow-cool-2)" : "url(#glow-warm-2)"}
              opacity={l.dim/100 * 0.85}/>
          ))}

          {/* Walls */}
          <g stroke={wallColor} strokeWidth="1.5" fill="none" strokeLinecap="square">
            <path d="M 20 40 L 220 40 L 220 200 L 20 200 Z"/>
            <path d="M 220 110 L 220 160" stroke="#0d0c08" strokeWidth="3"/>
          </g>
          <g stroke={wallColor} strokeWidth="1.5" fill="none">
            <path d="M 220 40 L 340 40 L 340 200 L 220 200"/>
          </g>
          <g stroke={wallColor} strokeWidth="1.5" fill="none">
            <path d="M 20 200 L 340 200 L 340 260 L 20 260 Z"/>
            <path d="M 80 200 L 110 200" stroke="#0d0c08" strokeWidth="3"/>
            <path d="M 230 200 L 260 200" stroke="#0d0c08" strokeWidth="3"/>
            <path d="M 80 260 L 110 260" stroke="#0d0c08" strokeWidth="3"/>
            <path d="M 240 260 L 270 260" stroke="#0d0c08" strokeWidth="3"/>
          </g>
          <g stroke={wallColor} strokeWidth="1.5" fill="none">
            <path d="M 20 260 L 200 260 L 200 400 L 20 400 Z"/>
          </g>
          <g stroke={wallColor} strokeWidth="1.5" fill="none">
            <path d="M 200 260 L 340 260 L 340 400 L 200 400 Z"/>
          </g>
          <g stroke={wallSubtle} strokeWidth="1" fill="none" strokeDasharray="3 3">
            <path d="M 260 400 L 340 400 L 340 450 L 260 450 Z"/>
          </g>

          {/* furniture */}
          <g stroke={wallSubtle} strokeWidth="1" fill="none">
            <rect x="30" y="160" width="80" height="28" rx="4"/>
            <line x1="50" y1="160" x2="50" y2="188"/>
            <line x1="70" y1="160" x2="70" y2="188"/>
            <line x1="90" y1="160" x2="90" y2="188"/>
            <rect x="140" y="50" width="60" height="6" rx="1"/>
            <rect x="240" y="80" width="80" height="20" rx="2"/>
            <rect x="260" y="135" width="60" height="40" rx="2"/>
            <rect x="40" y="290" width="100" height="80" rx="4"/>
            <line x1="40" y1="310" x2="140" y2="310"/>
            <rect x="220" y="280" width="40" height="60" rx="6"/>
            <circle cx="300" cy="310" r="14"/>
          </g>

          {[
            { x: 120, y: 32, t: "LIVING" },
            { x: 280, y: 32, t: "KITCHEN" },
            { x: 180, y: 254, t: "HALL" },
            { x: 110, y: 408, t: "BED" },
            { x: 270, y: 408, t: "BATH" },
            { x: 300, y: 458, t: "PORCH" },
          ].map(r => (
            <text key={r.t} x={r.x} y={r.y} textAnchor="middle"
              fill="rgba(245,189,110,0.55)" fontFamily="Geist Mono" fontSize="8"
              letterSpacing="2.4">{r.t}</text>
          ))}

          {/* dimensions */}
          <g stroke="rgba(245,189,110,0.25)" strokeWidth="0.5" fill="rgba(245,189,110,0.55)" fontFamily="Geist Mono" fontSize="7">
            <line x1="20" y1="20" x2="220" y2="20"/>
            <line x1="20" y1="14" x2="20" y2="26"/>
            <line x1="220" y1="14" x2="220" y2="26"/>
            <text x="120" y="14" textAnchor="middle">4.2m</text>
          </g>

          {/* lights */}
          {lights.map(l => (
            <g key={l.id}>
              <circle cx={l.x} cy={l.y} r="9" fill="none"
                stroke={l.on ? l.hue : "rgba(156,152,142,0.4)"}
                strokeWidth={l.on ? 1.5 : 1}/>
              <circle cx={l.x} cy={l.y} r={l.on ? 4 : 2}
                fill={l.on ? l.hue : "rgba(156,152,142,0.5)"}/>
              <text x={l.x + 13} y={l.y + 3}
                fill={l.on ? l.hue : "var(--text-dim)"}
                fontFamily="Geist Mono" fontSize="7" letterSpacing="0.4">
                {l.id}·{l.on ? l.dim : "off"}
              </text>
            </g>
          ))}

          <g transform="translate(330 432)">
            <circle cx="0" cy="0" r="12" fill="none" stroke="rgba(245,189,110,0.3)"/>
            <path d="M 0 -8 L 3 0 L 0 8 L -3 0 Z" fill="rgba(245,189,110,0.6)"/>
            <text x="0" y="-14" textAnchor="middle" fill="var(--on)" fontFamily="Geist Mono" fontSize="7">N</text>
          </g>
        </svg>
      </div>

      {/* active strip — sits above tab bar */}
      <div style={{ position: "absolute", left: 0, right: 0, bottom: 78, padding: "12px 18px 0",
        background: "linear-gradient(to top, #0d0c08 70%, rgba(13,12,8,0))" }}>
        <div className="mono" style={{ color: "var(--text-mute)", fontSize: 9, letterSpacing: "0.18em", marginBottom: 8 }}>
          ACTIVE ⌁ 04
        </div>
        <div style={{ display: "flex", gap: 6, overflowX: "auto", paddingBottom: 4 }}>
          {lights.filter(l => l.on).map(l => (
            <div key={l.id} style={{
              border: "1px solid rgba(245,189,110,0.25)",
              padding: "6px 9px", minWidth: 92, flexShrink: 0
            }}>
              <div className="mono" style={{ color: l.hue, fontSize: 9, letterSpacing: "0.1em" }}>{l.id}</div>
              <div style={{ fontSize: 11.5, fontWeight: 500, marginTop: 2 }}>{l.name}</div>
              <div className="mono" style={{ color: "var(--text-mute)", fontSize: 9.5, marginTop: 2 }}>
                {l.dim}% · {l.room}
              </div>
            </div>
          ))}
        </div>
      </div>

      <TabBar active="rooms"/>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// CONSOLE — adopted as a power-user page reached from
// Settings → System → Console.  No tab bar (focused mode).
// ─────────────────────────────────────────────────────────────

function ConsoleScreen() {
  const bar = (pct) => {
    const filled = Math.round(pct / 10);
    return "█".repeat(filled) + "░".repeat(10 - filled);
  };

  const devices = [
    { p: "rf",     ns: "living",  n: "floor-lamp",  st: "ON",  v: 62,  age: "10s" },
    { p: "matter", ns: "living",  n: "tv-strip",    st: "ON",  v: 28,  age: "2m"  },
    { p: "wifi",   ns: "kitchen", n: "island",      st: "ON",  v: 100, age: "1h"  },
    { p: "rf",     ns: "kitchen", n: "coffee-bar",  st: "--",  v: 0,   age: "3h"  },
    { p: "rf",     ns: "hall",    n: "hallway",     st: "ON",  v: 40,  age: "4m"  },
    { p: "matter", ns: "bed",     n: "nightstand",  st: "--",  v: 0,   age: "8h"  },
    { p: "matter", ns: "bed",     n: "ceiling",     st: "--",  v: 0,   age: "8h"  },
    { p: "wifi",   ns: "bath",    n: "mirror",      st: "--",  v: 0,   age: "9h"  },
    { p: "rf",     ns: "porch",   n: "porch",       st: "ON",  v: 80,  age: "1h"  },
  ];

  const events = [
    { t: "21:38:02", k: "ok",  m: "scene.evening triggered" },
    { t: "21:37:55", k: "set", m: "living/floor-lamp → 62%" },
    { t: "21:37:55", k: "set", m: "living/tv-strip → 28%"   },
    { t: "21:37:55", k: "set", m: "hall/hallway → 40%"      },
    { t: "21:14:00", k: "ok",  m: "porch turned on (sunset)" },
    { t: "20:02:33", k: "in",  m: "presence: home"          },
  ];

  const protocolColor = { rf: "var(--p-rf)", wifi: "var(--p-wifi)", matter: "var(--p-matter)", mqtt: "var(--p-mqtt)" };
  const evColor = { ok: "var(--good)", set: "var(--on)", in: "var(--cool)", err: "var(--bad)" };

  return (
    <div className="hh" style={{
      position: "relative", height: "100%", overflow: "hidden",
      background: "#0a0907", fontFamily: "var(--font-mono)",
      display: "flex", flexDirection: "column"
    }}>
      <div style={{ height: 54 }}/>

      {/* app-chrome header — back + title + connection indicator */}
      <div style={{ padding: "4px 16px 10px", display: "flex", justifyContent: "space-between", alignItems: "center",
        borderBottom: "1px solid rgba(245,189,110,0.2)", fontFamily: "var(--font-sans)" }}>
        <button className="chip" style={{ width: 36, height: 36, padding: 0, justifyContent: "center" }}>
          <Icon d={I.back} size={16} stroke={2}/>
        </button>
        <div style={{ textAlign: "center" }}>
          <div style={{ fontSize: 15, fontWeight: 600 }}>Console</div>
          <div className="mono" style={{ color: "var(--good)", fontSize: 10, letterSpacing: "0.1em", marginTop: 1 }}>
            ● homehub@pi-4 · live
          </div>
        </div>
        <button className="chip" style={{ width: 36, height: 36, padding: 0, justifyContent: "center" }}>
          <Icon d={I.more} size={16} stroke={2}/>
        </button>
      </div>

      {/* status box */}
      <div style={{ padding: "10px 16px 8px" }}>
        <div style={{ color: "var(--on)", fontSize: 11, letterSpacing: "0.04em" }}>
          $ status --watch
        </div>
        <div style={{ marginTop: 6, color: "var(--text)", fontSize: 11, lineHeight: 1.55 }}>
          ┌──────────────────────────────────────────┐<br/>
          │ <span style={{ color: "var(--on)" }}>04</span>/09 on · <span style={{ color: "var(--on)" }}>184W</span> · <span style={{ color: "var(--good)" }}>hub:up</span> · <span style={{ color: "var(--cool)" }}>net:ok</span>  │<br/>
          └──────────────────────────────────────────┘
        </div>
      </div>

      {/* devices table */}
      <div style={{ padding: "4px 16px 0", flex: 1, overflowY: "auto" }}>
        <div style={{ color: "var(--text-dim)", fontSize: 9, letterSpacing: "0.18em", marginBottom: 8 }}>
          # DEVICES ────────────────────────────
        </div>
        <div style={{ display: "grid", gridTemplateColumns: "14px 1fr 36px 84px 30px", gap: 8, fontSize: 9, color: "var(--text-dim)", letterSpacing: "0.1em", marginBottom: 6 }}>
          <span></span><span>HOST</span><span style={{ textAlign: "right" }}>VAL</span><span>LEVEL</span><span style={{ textAlign: "right" }}>AGE</span>
        </div>
        {devices.map((d, i) => {
          const on = d.st === "ON";
          return (
            <div key={i} style={{
              display: "grid", gridTemplateColumns: "14px 1fr 36px 84px 30px", gap: 8,
              padding: "5px 0", fontSize: 11,
              borderBottom: i < devices.length - 1 ? "1px dotted rgba(245,189,110,0.1)" : "none",
              alignItems: "center",
              color: on ? "var(--text)" : "var(--text-mute)"
            }}>
              <span style={{ color: on ? "var(--on)" : "var(--text-dim)" }}>{on ? "●" : "○"}</span>
              <span style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                <span style={{ color: protocolColor[d.p], fontSize: 9, letterSpacing: "0.1em" }}>{d.p.padEnd(6, " ")}</span>
                <span style={{ marginLeft: 6 }}>{d.ns}/{d.n}</span>
              </span>
              <span style={{ textAlign: "right", color: on ? "var(--on)" : "var(--text-dim)" }}>
                {on ? `${d.v}%` : "—"}
              </span>
              <span style={{ color: on ? "var(--on)" : "var(--text-dim)", letterSpacing: "-0.5px" }}>
                {bar(d.v)}
              </span>
              <span style={{ textAlign: "right", color: "var(--text-dim)", fontSize: 10 }}>{d.age}</span>
            </div>
          );
        })}

        <div style={{ color: "var(--text-dim)", fontSize: 9, letterSpacing: "0.18em", margin: "16px 0 8px" }}>
          # TAIL · LIVE ────────────────────────
        </div>
        {events.map((e, i) => (
          <div key={i} style={{ fontSize: 10.5, lineHeight: 1.6, color: "var(--text-mute)" }}>
            <span style={{ color: "var(--text-dim)" }}>{e.t}</span>
            <span style={{ color: evColor[e.k], margin: "0 8px" }}>{e.k.padEnd(3, " ")}</span>
            <span style={{ color: "var(--text)" }}>{e.m}</span>
          </div>
        ))}
        <div style={{ height: 100 }}/>
      </div>

      {/* command input */}
      <div style={{
        position: "absolute", left: 0, right: 0, bottom: 0,
        background: "#0a0907",
        borderTop: "1px solid rgba(245,189,110,0.25)",
        padding: "10px 16px 28px"
      }}>
        <div style={{ display: "flex", gap: 5, marginBottom: 10, overflowX: "auto" }}>
          {["all off", "scene evening", "scene goodnight", "porch on", "+5%"].map(c => (
            <span key={c} style={{
              flexShrink: 0,
              border: "1px solid rgba(245,189,110,0.3)",
              padding: "3px 8px", fontSize: 10, color: "var(--on)",
              letterSpacing: "0.04em"
            }}>{c}</span>
          ))}
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 13 }}>
          <span style={{ color: "var(--on)" }}>{"›"}</span>
          <span style={{ color: "var(--text-mute)", flex: 1 }}>set living/floor-lamp 80</span>
          <span style={{ width: 8, height: 14, background: "var(--on)",
            animation: "blink-2 1.1s steps(2) infinite" }}/>
        </div>
      </div>

      <style>{`@keyframes blink-2 { 50% { opacity: 0; } }`}</style>
    </div>
  );
}

Object.assign(window, { SpatialBlueprintScreen, ConsoleScreen, PageHeader });
