/* HomeHub — form / sheet / flow screens. */

// ── shared: bottom sheet shell ───────────────────────────────
function Sheet({ title, subtitle, children, primary = "Save", secondary, height }) {
  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "hidden", background: "rgba(0,0,0,0.5)" }}>
      <div style={{ height: 54 }}/>
      <div style={{ position: "absolute", left: 0, right: 0, bottom: 0, height: height || "82%", background: "var(--bg)", borderRadius: "28px 28px 0 0", border: "1px solid var(--hairline)", borderBottom: 0, display: "flex", flexDirection: "column", overflow: "hidden" }}>
        {/* grabber */}
        <div style={{ display: "grid", placeItems: "center", padding: "10px 0 4px" }}>
          <div style={{ width: 40, height: 4, borderRadius: 2, background: "var(--card-3)" }}/>
        </div>

        {/* header */}
        <div style={{ padding: "10px 22px 14px", display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 16 }}>
          <div>
            <h2 style={{ fontSize: 20, fontWeight: 600, letterSpacing: "-0.02em" }}>{title}</h2>
            {subtitle && <p style={{ color: "var(--text-mute)", fontSize: 13, marginTop: 4 }}>{subtitle}</p>}
          </div>
          <button style={{ width: 32, height: 32, borderRadius: "50%", background: "var(--card-3)", display: "grid", placeItems: "center" }}>
            <Icon d={I.close} size={14} stroke={2} style={{ color: "var(--text-mute)" }}/>
          </button>
        </div>

        {/* scroll body */}
        <div style={{ flex: 1, overflow: "auto", padding: "0 22px 0", display: "flex", flexDirection: "column", gap: 14 }}>
          {children}
        </div>

        {/* footer actions */}
        <div style={{ padding: "16px 22px 28px", display: "flex", gap: 10, borderTop: "1px solid var(--hairline)", background: "var(--bg)" }}>
          {secondary && (
            <button style={{ flex: 1, padding: "14px", borderRadius: 14, background: "var(--card)", border: "1px solid var(--hairline)", color: "var(--text)", fontSize: 14, fontWeight: 500 }}>
              {secondary}
            </button>
          )}
          <button style={{ flex: 2, padding: "14px", borderRadius: 22, background: "var(--on)", color: "#3a2400", fontSize: 14, fontWeight: 600 }}>
            {primary}
          </button>
        </div>
      </div>
    </div>
  );
}

// ── ADD/EDIT SCHEDULE ───────────────────────────────────────
function ScheduleFormScreen() {
  const days = ["S", "M", "T", "W", "T", "F", "S"];
  const enabled = [false, true, true, true, true, true, false];
  return (
    <Sheet title="New schedule" subtitle="Run a device, scene, or group at a time">
      <Field label="Trigger">
        <div className="card" style={{ padding: "12px 14px", flexDirection: "row", display: "flex", alignItems: "center", gap: 10 }}>
          <div style={{ width: 28, height: 28, borderRadius: 8, background: "var(--card-3)", display: "grid", placeItems: "center" }}>
            <Icon d={I.scenes} size={14} stroke={1.7} style={{ color: "var(--on)" }}/>
          </div>
          <span style={{ fontSize: 14, fontWeight: 500 }}>Wake up scene</span>
          <span style={{ marginLeft: "auto", color: "var(--text-mute)", fontSize: 12.5 }}>scene</span>
          <Icon d={I.chevR} size={14} stroke={1.7} style={{ color: "var(--text-dim)" }}/>
        </div>
      </Field>

      <Field label="When">
        <div className="card" style={{ padding: 4, flexDirection: "row", display: "flex", gap: 4 }}>
          {[
            { v: "fixed",   l: "At time", a: true },
            { v: "sunrise", l: "Sunrise" },
            { v: "sunset",  l: "Sunset" },
          ].map(o => (
            <button key={o.v} style={{ flex: 1, padding: "10px 0", borderRadius: 10, background: o.a ? "var(--card-3)" : "transparent", color: o.a ? "var(--text)" : "var(--text-mute)", fontWeight: 500, fontSize: 13 }}>
              {o.l}
            </button>
          ))}
        </div>
      </Field>

      <Field label="Time">
        <div className="card" style={{ padding: "18px 14px", flexDirection: "row", display: "flex", alignItems: "baseline", justifyContent: "center", gap: 4 }}>
          <span className="num-display" style={{ fontSize: 44, color: "var(--text)" }}>07</span>
          <span className="num-display" style={{ fontSize: 44, color: "var(--text-dim)" }}>:</span>
          <span className="num-display" style={{ fontSize: 44, color: "var(--text)" }}>00</span>
        </div>
      </Field>

      <Field label="Repeat" help="Tap days to toggle">
        <div style={{ display: "flex", gap: 6, justifyContent: "space-between" }}>
          {days.map((d, i) => (
            <div key={i} style={{
              width: 38, height: 38, borderRadius: "50%",
              background: enabled[i] ? "var(--on)" : "var(--card)",
              border: enabled[i] ? "0" : "1px solid var(--hairline)",
              color: enabled[i] ? "#3a2400" : "var(--text-mute)",
              display: "grid", placeItems: "center",
              fontSize: 13, fontWeight: 600
            }}>
              {d}
            </div>
          ))}
        </div>
      </Field>

      <Field label="Options">
        <div className="card" style={{ padding: 0, overflow: "hidden" }}>
          <Toggle label="Random offset" sub="Up to ±10 minutes" on={false}/>
          <div className="sep" style={{ marginLeft: 16 }}/>
          <Toggle label="Skip when away" sub="Pauses while Away scene is active" on={true}/>
        </div>
      </Field>
    </Sheet>
  );
}

function Toggle({ label, sub, on }) {
  return (
    <div style={{ display: "flex", alignItems: "center", padding: "12px 16px", gap: 12 }}>
      <div style={{ flex: 1 }}>
        <div style={{ fontSize: 14, fontWeight: 500 }}>{label}</div>
        {sub && <div style={{ color: "var(--text-mute)", fontSize: 12, marginTop: 2 }}>{sub}</div>}
      </div>
      <div className={`sw ${on ? "on" : ""}`}/>
    </div>
  );
}

// ── ADD/EDIT SCENE ──────────────────────────────────────────
function SceneFormScreen() {
  const actions = [
    { name: "Floor lamp",   action: "on",  dim: 60 },
    { name: "TV strip",     action: "on",  dim: 30 },
    { name: "Sofa lamp",    action: "off" },
    { name: "Kitchen isle", action: "on",  dim: 80 },
    { name: "Under cabinet", action: "off" },
    { name: "Bedroom main", action: "off" },
  ];
  return (
    <Sheet title="New scene" subtitle="Set multiple devices in one tap">
      <Field label="Name">
        <input defaultValue="Evening" style={inputStyle}/>
      </Field>

      <Field label="Color">
        <div style={{ display: "flex", gap: 10 }}>
          {["var(--on)", "var(--cool)", "#a96bd9", "#d97a45", "#ffd066", "var(--good)"].map((c, i) => (
            <div key={i} style={{ width: 36, height: 36, borderRadius: "50%", background: c, border: i === 0 ? "3px solid var(--text)" : "0", boxShadow: i === 0 ? "0 0 0 2px var(--on)" : "none" }}/>
          ))}
        </div>
      </Field>

      <Field label={`Devices · ${actions.length}`} help="Tap a row to remove. Use + Add to include more.">
        <div className="card" style={{ padding: 0, overflow: "hidden" }}>
          {actions.map((a, i, arr) => (
            <React.Fragment key={a.name}>
              <div style={{ display: "flex", alignItems: "center", padding: "12px 16px", gap: 12 }}>
                <div className="tile-bulb" style={{ width: 28, height: 28, background: a.action === "on" ? "var(--on)" : "var(--card-3)" }}>
                  <Icon d={I.bulb} size={13} stroke={1.7} style={{ color: a.action === "on" ? "#3a2400" : "var(--text-mute)" }}/>
                </div>
                <div style={{ flex: 1 }}>
                  <div style={{ fontSize: 14, fontWeight: 500 }}>{a.name}</div>
                  <div style={{ color: "var(--text-mute)", fontSize: 12, marginTop: 2 }}>
                    {a.action === "on" ? (a.dim != null ? `On · ${a.dim}%` : "On") : "Off"}
                  </div>
                </div>
                <div className={`sw ${a.action === "on" ? "on" : ""}`}/>
              </div>
              {i < arr.length - 1 && <div className="sep" style={{ marginLeft: 56 }}/>}
            </React.Fragment>
          ))}
          <div style={{ padding: "12px 16px", borderTop: "1px solid var(--hairline)" }}>
            <button style={{ color: "var(--on)", fontSize: 14, fontWeight: 500, display: "flex", alignItems: "center", gap: 8 }}>
              <Icon d={I.plus} size={14} stroke={2}/> Add device
            </button>
          </div>
        </div>
      </Field>
    </Sheet>
  );
}

// ── ADD DEVICE — protocol picker ────────────────────────────
function AddDeviceScreen() {
  const protocols = [
    { kind: "matter", title: "Matter device",       sub: "Scan a QR code on the device", color: "var(--p-matter)" },
    { kind: "wifi",   title: "Wi-Fi (Tasmota)",     sub: "Enter the device IP",          color: "var(--p-wifi)" },
    { kind: "rf",     title: "433 MHz socket",      sub: "Learn from a remote",          color: "var(--p-rf)" },
    { kind: "mqtt",   title: "MQTT device",         sub: "Publish to a topic",           color: "var(--p-mqtt)" },
  ];
  return (
    <Sheet title="Add a device" subtitle="What kind of device are you adding?" height="68%">
      <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
        {protocols.map(p => (
          <button key={p.kind} className="card" style={{ padding: 16, flexDirection: "row", display: "flex", alignItems: "center", gap: 14, textAlign: "left", width: "100%" }}>
            <div style={{ width: 44, height: 44, borderRadius: 12, background: "var(--card-3)", display: "grid", placeItems: "center", flexShrink: 0 }}>
              <Icon d={I[p.kind] || I.matter} size={20} stroke={1.7} style={{ color: p.color }}/>
            </div>
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{ fontSize: 15, fontWeight: 600 }}>{p.title}</div>
              <div style={{ color: "var(--text-mute)", fontSize: 12.5, marginTop: 2 }}>{p.sub}</div>
            </div>
            <Icon d={I.chevR} size={14} stroke={1.7} style={{ color: "var(--text-dim)" }}/>
          </button>
        ))}
      </div>
    </Sheet>
  );
}

// ── MATTER COMMISSIONING — multi-step flow ──────────────────
function MatterStep1QR() {
  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "hidden", background: "#0a0a08" }}>
      <div style={{ height: 54 }}/>
      {/* fake camera viewfinder */}
      <div style={{ position: "absolute", inset: "54px 0 0 0", background: "radial-gradient(circle at center, #2a2a2a 0%, #0a0a08 80%)" }}/>
      {/* QR cutout */}
      <div style={{ position: "absolute", left: "50%", top: "44%", transform: "translate(-50%, -50%)", width: 240, height: 240, borderRadius: 24, boxShadow: "0 0 0 9999px rgba(0,0,0,0.55)" }}/>
      {/* corner brackets */}
      {[
        { t: "44%", l: "50%", x: -120, y: -120, rot: 0 },
        { t: "44%", l: "50%", x: 80, y: -120, rot: 90 },
        { t: "44%", l: "50%", x: 80, y: 80, rot: 180 },
        { t: "44%", l: "50%", x: -120, y: 80, rot: 270 },
      ].map((c, i) => (
        <div key={i} style={{ position: "absolute", top: c.t, left: c.l, transform: `translate(-50%, -50%) translate(${c.x}px, ${c.y}px) rotate(${c.rot}deg)`, width: 40, height: 40, borderTop: "3px solid var(--on)", borderLeft: "3px solid var(--on)", borderTopLeftRadius: 12 }}/>
      ))}
      {/* scanning line */}
      <div style={{ position: "absolute", top: "44%", left: "50%", transform: "translate(-50%, -50%)", width: 220, height: 2, background: "linear-gradient(90deg, transparent, var(--on), transparent)", borderRadius: 2, boxShadow: "0 0 12px var(--on-glow)" }}/>

      {/* header */}
      <div style={{ position: "absolute", top: 54, left: 0, right: 0, padding: "8px 22px", display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <button style={{ width: 36, height: 36, borderRadius: "50%", background: "rgba(0,0,0,0.55)", display: "grid", placeItems: "center", color: "#fff" }}>
          <Icon d={I.close} size={16} stroke={2}/>
        </button>
        <div style={{ background: "rgba(0,0,0,0.55)", padding: "6px 12px", borderRadius: 999, color: "#fff", fontSize: 12, fontFamily: "var(--font-mono)" }}>
          Step 1 of 4
        </div>
        <button style={{ width: 36, height: 36, borderRadius: "50%", background: "rgba(0,0,0,0.55)", display: "grid", placeItems: "center", color: "#fff" }}>
          <Icon d={I.settings} size={16} stroke={1.7}/>
        </button>
      </div>

      {/* bottom panel */}
      <div style={{ position: "absolute", left: 0, right: 0, bottom: 0, padding: "20px 22px 40px", background: "linear-gradient(to top, rgba(10,10,8,0.95) 60%, transparent)", color: "#fff" }}>
        <div style={{ fontSize: 22, fontWeight: 600, letterSpacing: "-0.02em" }}>Scan setup code</div>
        <div style={{ color: "rgba(255,255,255,0.65)", fontSize: 13, marginTop: 6, lineHeight: 1.4 }}>
          Find the Matter QR code on the device, packaging, or in the manufacturer's app.
        </div>
        <button style={{ marginTop: 18, width: "100%", padding: "14px", borderRadius: 16, background: "rgba(255,255,255,0.12)", color: "#fff", fontWeight: 500, fontSize: 14 }}>
          Enter 11-digit code instead
        </button>
      </div>
    </div>
  );
}

function MatterStep2Connecting() {
  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "hidden", display: "flex", flexDirection: "column" }}>
      <div style={{ height: 54 }}/>

      <div style={{ padding: "8px 22px", display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <button style={{ color: "var(--text-mute)", fontSize: 14 }}>← Back</button>
        <div className="chip" style={{ padding: "4px 10px", fontSize: 11 }}>Step 2 of 4</div>
        <div style={{ width: 50 }}/>
      </div>

      <div style={{ padding: "40px 22px 0", flex: 1, display: "flex", flexDirection: "column", alignItems: "center", textAlign: "center" }}>
        {/* pulsing brand */}
        <div style={{ position: "relative", width: 120, height: 120, marginBottom: 30, display: "grid", placeItems: "center" }}>
          <div style={{ position: "absolute", inset: 0, borderRadius: "50%", border: "2px solid var(--on)", opacity: 0.3 }}/>
          <div style={{ position: "absolute", inset: 15, borderRadius: "50%", border: "2px solid var(--on)", opacity: 0.5 }}/>
          <div style={{ width: 56, height: 56, borderRadius: 16, background: "var(--on)", display: "grid", placeItems: "center" }}>
            <Icon d={I.matter} size={26} stroke={1.7} style={{ color: "#3a2400" }}/>
          </div>
        </div>

        <h1 style={{ fontSize: 24, fontWeight: 600, letterSpacing: "-0.02em" }}>Commissioning device…</h1>
        <p style={{ color: "var(--text-mute)", fontSize: 14, marginTop: 8, maxWidth: 260, lineHeight: 1.45 }}>
          Keep the device powered on and within Bluetooth range. This can take up to 60 seconds.
        </p>
      </div>

      <div style={{ padding: "0 22px 30px" }}>
        <div className="card" style={{ padding: 16, gap: 0 }}>
          {[
            { l: "Bluetooth handshake", state: "done" },
            { l: "Verify device",       state: "done" },
            { l: "Join Wi-Fi network",  state: "doing" },
            { l: "Operational certificate", state: "todo" },
          ].map((s, i) => (
            <div key={i} style={{ display: "flex", alignItems: "center", padding: "10px 0", gap: 12 }}>
              <div style={{ width: 20, height: 20, borderRadius: "50%", background: s.state === "done" ? "var(--on)" : (s.state === "doing" ? "var(--on-soft)" : "var(--card-3)"), border: s.state === "doing" ? "2px solid var(--on)" : "0", display: "grid", placeItems: "center" }}>
                {s.state === "done" && <Icon d="M5 12l4 4L19 7" size={10} stroke={3} style={{ color: "#3a2400" }}/>}
              </div>
              <span style={{ fontSize: 13.5, color: s.state === "todo" ? "var(--text-mute)" : "var(--text)", fontWeight: s.state === "doing" ? 600 : 400 }}>{s.l}</span>
              {s.state === "doing" && <span className="mono" style={{ marginLeft: "auto", fontSize: 11, color: "var(--on)" }}>in progress</span>}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function MatterStep3WiFi() {
  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "hidden", display: "flex", flexDirection: "column" }}>
      <div style={{ height: 54 }}/>

      <div style={{ padding: "8px 22px", display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <button style={{ color: "var(--text-mute)", fontSize: 14 }}>← Back</button>
        <div className="chip" style={{ padding: "4px 10px", fontSize: 11 }}>Step 3 of 4</div>
        <div style={{ width: 50 }}/>
      </div>

      <div style={{ padding: "24px 22px 0" }}>
        <h1 style={{ fontSize: 24, fontWeight: 600, letterSpacing: "-0.02em" }}>Wi-Fi for the device</h1>
        <p style={{ color: "var(--text-mute)", fontSize: 14, marginTop: 8, lineHeight: 1.45 }}>
          The device needs the same Wi-Fi credentials your hub uses. We'll send them over Bluetooth.
        </p>
      </div>

      <div style={{ padding: "28px 22px 0", display: "flex", flexDirection: "column", gap: 14 }}>
        <Field label="Network">
          <div className="card" style={{ padding: "14px 16px", flexDirection: "row", display: "flex", alignItems: "center", gap: 12 }}>
            <Icon d={I.wifi} size={18} stroke={1.7} style={{ color: "var(--p-wifi)" }}/>
            <div style={{ flex: 1 }}>
              <div style={{ fontSize: 14, fontWeight: 500 }}>HomeMesh 5G</div>
              <div style={{ color: "var(--text-mute)", fontSize: 11.5, marginTop: 2, fontFamily: "var(--font-mono)" }}>WPA2 · −48 dBm</div>
            </div>
            <button style={{ color: "var(--cool)", fontSize: 13, fontWeight: 500 }}>Change</button>
          </div>
        </Field>

        <Field label="Password">
          <div style={{ position: "relative" }}>
            <input type="password" defaultValue="••••••••••••" style={inputStyle}/>
            <button style={{ position: "absolute", right: 10, top: "50%", transform: "translateY(-50%)", color: "var(--text-mute)", fontSize: 12, padding: "4px 8px", borderRadius: 8, background: "var(--card-3)" }}>Show</button>
          </div>
        </Field>

        <div className="card" style={{ padding: 14, flexDirection: "row", display: "flex", gap: 12, alignItems: "flex-start", background: "var(--card-2)", borderColor: "var(--hairline)" }}>
          <div style={{ width: 32, height: 32, borderRadius: 8, background: "var(--card-3)", display: "grid", placeItems: "center", flexShrink: 0 }}>
            <Icon d={I.bell} size={15} stroke={1.7} style={{ color: "var(--cool)" }}/>
          </div>
          <div style={{ fontSize: 12.5, color: "var(--text-mute)", lineHeight: 1.45 }}>
            Credentials never leave your phone in plain text. They're encrypted end-to-end to the device.
          </div>
        </div>

        <Field label="Options">
          <div className="card" style={{ padding: 0, overflow: "hidden" }}>
            <Toggle label="Use 2.4 GHz band" sub="Recommended for low-power devices" on={true}/>
            <div className="sep" style={{ marginLeft: 16 }}/>
            <Toggle label="Static IP" sub="For long-lived sensors" on={false}/>
          </div>
        </Field>
      </div>

      <div style={{ marginTop: "auto", padding: "0 22px 30px" }}>
        <button style={{ width: "100%", padding: "14px", borderRadius: 22, background: "var(--on)", color: "#3a2400", fontSize: 14, fontWeight: 600 }}>
          Send to device
        </button>
      </div>
    </div>
  );
}

function MatterStep4Done() {
  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "hidden", display: "flex", flexDirection: "column" }}>
      <div style={{ height: 54 }}/>

      <div style={{ padding: "8px 22px", display: "flex", justifyContent: "center", alignItems: "center" }}>
        <div className="chip" style={{ padding: "4px 10px", fontSize: 11 }}>Step 4 of 4</div>
      </div>

      <div style={{ padding: "32px 22px 0", textAlign: "center" }}>
        <div style={{ display: "inline-grid", placeItems: "center", width: 88, height: 88, borderRadius: "50%", background: "var(--on)", boxShadow: "0 0 0 8px var(--on-soft), 0 0 32px var(--on-glow)" }}>
          <Icon d="M5 12l5 5L20 7" size={36} stroke={3} style={{ color: "#3a2400" }}/>
        </div>
        <h1 style={{ fontSize: 26, fontWeight: 600, marginTop: 22, letterSpacing: "-0.02em" }}>Added!</h1>
        <p style={{ color: "var(--text-mute)", fontSize: 14, marginTop: 8 }}>Name the new device and place it in a room.</p>
      </div>

      <div style={{ padding: "32px 22px 0", display: "flex", flexDirection: "column", gap: 14 }}>
        <Field label="Name">
          <input defaultValue="Living room bulb 3" style={inputStyle}/>
        </Field>
        <Field label="Room">
          <div className="card" style={{ padding: "12px 14px", flexDirection: "row", display: "flex", alignItems: "center", gap: 10 }}>
            <span style={{ fontSize: 14, flex: 1, fontWeight: 500 }}>Living room</span>
            <Icon d={I.chevD} size={14} stroke={1.7} style={{ color: "var(--text-dim)" }}/>
          </div>
        </Field>

        <div className="card" style={{ padding: 14, flexDirection: "row", display: "flex", alignItems: "center", gap: 12, marginTop: 6 }}>
          <div style={{ width: 36, height: 36, borderRadius: 8, background: "var(--card-3)", display: "grid", placeItems: "center" }}>
            <Icon d={I.matter} size={17} stroke={1.7} style={{ color: "var(--p-matter)" }}/>
          </div>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ fontSize: 12, color: "var(--text-mute)", fontFamily: "var(--font-mono)", letterSpacing: "0.05em" }}>Signify · LCA001</div>
            <div style={{ fontSize: 13, fontWeight: 500, marginTop: 2 }}>Color bulb · brightness, color, temperature</div>
          </div>
        </div>
      </div>

      <div style={{ marginTop: "auto", padding: "0 22px 30px", display: "flex", gap: 10 }}>
        <button style={{ flex: 1, padding: "14px", borderRadius: 14, background: "var(--card)", border: "1px solid var(--hairline)", color: "var(--text)", fontSize: 14, fontWeight: 500 }}>
          Add another
        </button>
        <button style={{ flex: 2, padding: "14px", borderRadius: 22, background: "var(--on)", color: "#3a2400", fontSize: 14, fontWeight: 600 }}>
          Done
        </button>
      </div>
    </div>
  );
}

// ── EDIT FORMS ─────────────────────────────────────────────

function GroupFormScreen() {
  const members = [
    { name: "Floor lamp",    room: "Living",  included: true },
    { name: "TV strip",      room: "Living",  included: true },
    { name: "Sofa lamp",     room: "Living",  included: true },
    { name: "Reading nook",  room: "Living",  included: false },
    { name: "Kitchen isle",  room: "Kitchen", included: true },
    { name: "Under cabinet", room: "Kitchen", included: false },
    { name: "Coffee bar",    room: "Kitchen", included: false },
    { name: "Hallway",       room: "Hall",    included: false },
  ];
  return (
    <Sheet title="Edit group" subtitle="Control multiple devices in one call">
      <Field label="Name">
        <input defaultValue="Downstairs" style={inputStyle}/>
      </Field>
      <Field label={`Members · ${members.filter(m => m.included).length} selected`}>
        <div className="card" style={{ padding: 0, overflow: "hidden" }}>
          {members.map((m, i, arr) => (
            <React.Fragment key={m.name}>
              <label style={{ display: "flex", alignItems: "center", padding: "12px 16px", gap: 12, cursor: "pointer" }}>
                <div style={{ width: 22, height: 22, borderRadius: 6, background: m.included ? "var(--on)" : "transparent", border: m.included ? "0" : "1.5px solid var(--border)", display: "grid", placeItems: "center", flexShrink: 0 }}>
                  {m.included && <Icon d="M4 10l4 4 8-8" size={12} stroke={3} style={{ color: "#3a2400" }}/>}
                </div>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ fontSize: 14, fontWeight: 500 }}>{m.name}</div>
                  <div style={{ color: "var(--text-mute)", fontSize: 12, marginTop: 2 }}>{m.room}</div>
                </div>
              </label>
              {i < arr.length - 1 && <div className="sep" style={{ marginLeft: 50 }}/>}
            </React.Fragment>
          ))}
        </div>
      </Field>
    </Sheet>
  );
}

function SensorFormScreen() {
  return (
    <Sheet title="Edit sensor" subtitle="Bedroom · Temperature" primary="Save" secondary="Delete">
      <Field label="Name">
        <input defaultValue="Bedroom" style={inputStyle}/>
      </Field>
      <Field label="Kind">
        <div className="card" style={{ padding: 4, flexDirection: "row", display: "flex", gap: 4, flexWrap: "wrap" }}>
          {[
            { v: "temp", l: "Temperature", a: true },
            { v: "humid", l: "Humidity" },
            { v: "motion", l: "Motion" },
            { v: "power", l: "Power" },
          ].map(o => (
            <button key={o.v} style={{ flex: "1 1 40%", padding: "10px 0", borderRadius: 10, background: o.a ? "var(--card-3)" : "transparent", color: o.a ? "var(--text)" : "var(--text-mute)", fontWeight: 500, fontSize: 13 }}>
              {o.l}
            </button>
          ))}
        </div>
      </Field>
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 14 }}>
        <Field label="Unit"><input defaultValue="°C" style={inputStyle}/></Field>
        <Field label="Room">
          <div className="card" style={{ padding: "14px 16px", display: "flex", alignItems: "center" }}>
            <span style={{ fontSize: 14, flex: 1 }}>Bedroom</span>
            <Icon d={I.chevD} size={14} stroke={1.7} style={{ color: "var(--text-dim)" }}/>
          </div>
        </Field>
      </div>
      <Field label="Source">
        <div className="card" style={{ padding: "14px 16px", flexDirection: "row", display: "flex", alignItems: "center", gap: 10 }}>
          <ProtocolBadge kind="matter"/>
          <span style={{ fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--text-mute)", marginLeft: 4 }}>node-04a · temp</span>
        </div>
      </Field>
      <Field label="Alert thresholds" help="Get a push when value crosses these limits">
        <div className="card" style={{ padding: 14, gap: 12 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
            <span style={{ fontSize: 13, color: "var(--text-mute)", width: 60 }}>Min</span>
            <div className="rail" style={{ flex: 1, height: 6 }}><i style={{ width: "20%" }}/></div>
            <span className="mono" style={{ fontSize: 13, color: "var(--bad)", width: 50, textAlign: "right" }}>18.0°</span>
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
            <span style={{ fontSize: 13, color: "var(--text-mute)", width: 60 }}>Max</span>
            <div className="rail" style={{ flex: 1, height: 6 }}><i style={{ width: "75%" }}/></div>
            <span className="mono" style={{ fontSize: 13, color: "var(--on)", width: 50, textAlign: "right" }}>26.0°</span>
          </div>
        </div>
      </Field>
    </Sheet>
  );
}

function SocketFormScreen() {
  return (
    <Sheet title="Edit device" subtitle="Floor lamp · Living room" primary="Save" secondary="Delete">
      <Field label="Name"><input defaultValue="Floor lamp" style={inputStyle}/></Field>
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 14 }}>
        <Field label="Room">
          <div className="card" style={{ padding: "14px 16px", display: "flex", alignItems: "center" }}>
            <span style={{ fontSize: 14, flex: 1 }}>Living room</span>
            <Icon d={I.chevD} size={14} stroke={1.7} style={{ color: "var(--text-dim)" }}/>
          </div>
        </Field>
        <Field label="Favorite">
          <div className="card" style={{ padding: "14px 16px", display: "flex", alignItems: "center" }}>
            <Icon d={I.star} size={14} stroke={1.7} style={{ color: "var(--on)", marginRight: 8 }}/>
            <span style={{ fontSize: 14, flex: 1 }}>Pinned</span>
            <div className="sw on"/>
          </div>
        </Field>
      </div>
      <Field label="Protocol">
        <div className="card" style={{ padding: "14px 16px", flexDirection: "row", display: "flex", alignItems: "center", gap: 10 }}>
          <ProtocolBadge kind="matter"/>
          <span style={{ marginLeft: 4, fontSize: 13, color: "var(--text)" }}>Matter</span>
          <span style={{ marginLeft: "auto", fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--text-mute)" }}>node-12c</span>
          <Icon d={I.chevR} size={14} stroke={1.7} style={{ color: "var(--text-dim)" }}/>
        </div>
      </Field>
      <Field label="Capabilities">
        <div className="card" style={{ padding: 0, overflow: "hidden" }}>
          <Toggle label="Brightness" sub="0–100%" on={true}/>
          <div className="sep" style={{ marginLeft: 16 }}/>
          <Toggle label="Color" sub="RGB" on={true}/>
          <div className="sep" style={{ marginLeft: 16 }}/>
          <Toggle label="Color temperature" sub="2700–6500 K" on={true}/>
        </div>
      </Field>
      <Field label="Diagnostics">
        <div className="card" style={{ padding: 0, overflow: "hidden" }}>
          {[
            { l: "Last seen", v: "2 seconds ago" },
            { l: "Signal",    v: "−48 dBm", c: "var(--good)" },
            { l: "Firmware",  v: "1.42.3" },
          ].map((r, i, a) => (
            <React.Fragment key={r.l}>
              <div style={{ display: "flex", padding: "12px 16px", justifyContent: "space-between", alignItems: "center" }}>
                <span style={{ fontSize: 13, color: "var(--text-mute)" }}>{r.l}</span>
                <span className="mono" style={{ fontSize: 13, color: r.c || "var(--text)" }}>{r.v}</span>
              </div>
              {i < a.length - 1 && <div className="sep" style={{ marginLeft: 16 }}/>}
            </React.Fragment>
          ))}
        </div>
      </Field>
    </Sheet>
  );
}

Object.assign(window, {
  Sheet, Toggle,
  ScheduleFormScreen, SceneFormScreen, AddDeviceScreen,
  MatterStep1QR, MatterStep2Connecting, MatterStep3WiFi, MatterStep4Done,
  GroupFormScreen, SensorFormScreen, SocketFormScreen,
});
