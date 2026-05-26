/* HomeHub — modal states, toasts, empty states, confirmations. */

// ── CONFIRM MODAL ───────────────────────────────────────────
function ConfirmModalScreen() {
  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "hidden", background: "rgba(0,0,0,0.55)", display: "grid", placeItems: "center", padding: 22 }}>
      <div className="card" style={{ padding: 24, gap: 16, width: "100%", maxWidth: 340, background: "var(--bg-2)" }}>
        <div style={{ width: 48, height: 48, borderRadius: 14, background: "rgba(224,138,122,0.14)", display: "grid", placeItems: "center", margin: "0 auto" }}>
          <Icon d={I.bell} size={22} stroke={1.7} style={{ color: "var(--bad)" }}/>
        </div>
        <div style={{ textAlign: "center" }}>
          <h2 style={{ fontSize: 18, fontWeight: 600 }}>Delete schedule?</h2>
          <p style={{ color: "var(--text-mute)", fontSize: 13.5, marginTop: 8, lineHeight: 1.45 }}>
            "Coffee bar weekdays" will stop running and cannot be undone.
          </p>
        </div>
        <div style={{ display: "flex", gap: 10 }}>
          <button style={{ flex: 1, padding: "12px", borderRadius: 14, background: "var(--card)", border: "1px solid var(--hairline)", color: "var(--text)", fontSize: 14, fontWeight: 500 }}>
            Cancel
          </button>
          <button style={{ flex: 1, padding: "12px", borderRadius: 14, background: "var(--bad)", color: "#fff", fontSize: 14, fontWeight: 600 }}>
            Delete
          </button>
        </div>
      </div>
    </div>
  );
}

// ── TOAST STACK ────────────────────────────────────────────
function ToastsScreen() {
  const toasts = [
    { tone: "success", title: "Scene activated",        body: "Evening · 8 devices",     icon: I.scenes },
    { tone: "info",    title: "Update ready",           body: "A new version is available.", icon: I.bell, action: "Refresh" },
    { tone: "warn",    title: "Sensor lost signal",     body: "Hallway · 12 minutes",    icon: I.bell },
    { tone: "error",   title: "Schedule failed",        body: "Floor lamp did not respond", icon: I.power, action: "Retry" },
  ];
  const c = (t) =>
    t === "success" ? "var(--good)" :
    t === "warn"    ? "#e8b96b" :
    t === "error"   ? "var(--bad)"  : "var(--cool)";

  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "hidden", background: "var(--bg)" }}>
      <div style={{ height: 54 }}/>

      <div style={{ padding: "8px 22px 0" }}>
        <h1 style={{ fontSize: 26, fontWeight: 600 }}>Toasts</h1>
        <p style={{ color: "var(--text-mute)", fontSize: 13, marginTop: 4 }}>
          Four tones — left-edge accent + colored icon. Appears bottom-right (desktop) / above tab bar (mobile).
        </p>
      </div>

      <div style={{ padding: "30px 22px 0", display: "flex", flexDirection: "column", gap: 10 }}>
        {toasts.map((t, i) => (
          <div key={i} className="card" style={{ padding: "14px 14px 14px 12px", flexDirection: "row", display: "flex", gap: 12, alignItems: "flex-start", borderLeft: `3px solid ${c(t.tone)}` }}>
            <div style={{ width: 32, height: 32, borderRadius: 10, background: "var(--card-3)", display: "grid", placeItems: "center", flexShrink: 0 }}>
              <Icon d={t.icon} size={15} stroke={1.7} style={{ color: c(t.tone) }}/>
            </div>
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{ fontWeight: 600, fontSize: 14 }}>{t.title}</div>
              <div style={{ color: "var(--text-mute)", fontSize: 12.5, marginTop: 2 }}>{t.body}</div>
            </div>
            {t.action && (
              <button style={{ padding: "6px 10px", borderRadius: 8, border: "1px solid var(--border)", color: "var(--text)", fontSize: 12, fontWeight: 600, flexShrink: 0 }}>
                {t.action}
              </button>
            )}
            <button style={{ color: "var(--text-dim)", fontSize: 18, lineHeight: 1, padding: 0, flexShrink: 0 }}>×</button>
          </div>
        ))}
      </div>
    </div>
  );
}

// ── EMPTY STATES ───────────────────────────────────────────
function EmptyStatesScreen() {
  const states = [
    { d: I.bulb,     title: "No devices yet",        sub: "Add your first plug, bulb, or switch to get started.", cta: "Add device" },
    { d: I.scenes,   title: "No scenes yet",         sub: "Group device states into one tap.",                     cta: "Create scene" },
    { d: I.schedule, title: "Nothing scheduled",     sub: "Automate sunset, mornings, away mode.",                 cta: "New schedule" },
    { d: I.sensor,   title: "No sensors paired",     sub: "Pair a Matter or MQTT sensor to track conditions.",     cta: "Pair sensor" },
  ];
  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "hidden", background: "var(--bg)" }}>
      <div style={{ height: 54 }}/>

      <div style={{ padding: "8px 22px 0" }}>
        <h1 style={{ fontSize: 26, fontWeight: 600 }}>Empty states</h1>
        <p style={{ color: "var(--text-mute)", fontSize: 13, marginTop: 4 }}>
          Used in every list view when there's nothing to show.
        </p>
      </div>

      <div style={{ padding: "20px 22px 0", display: "grid", gridTemplateColumns: "1fr 1fr", gap: 10 }}>
        {states.map((e, i) => (
          <div key={i} className="card" style={{ padding: 18, gap: 12, alignItems: "center", textAlign: "center", border: "1px dashed var(--border)", background: "var(--card-2)" }}>
            <div style={{ width: 44, height: 44, borderRadius: "50%", background: "var(--card-3)", display: "grid", placeItems: "center" }}>
              <Icon d={e.d} size={20} stroke={1.7} style={{ color: "var(--text-mute)" }}/>
            </div>
            <div>
              <div style={{ fontSize: 14, fontWeight: 600 }}>{e.title}</div>
              <div style={{ color: "var(--text-mute)", fontSize: 11.5, marginTop: 4, lineHeight: 1.35 }}>{e.sub}</div>
            </div>
            <button style={{ padding: "8px 12px", borderRadius: 10, background: "var(--on-soft)", color: "var(--on)", fontSize: 12, fontWeight: 600 }}>
              {e.cta}
            </button>
          </div>
        ))}
      </div>
    </div>
  );
}

// ── TIMER (one-shot) MODAL ─────────────────────────────────
function TimerSheet() {
  const minutes = 30;
  return (
    <Sheet title="Off in…" subtitle="One-shot timer for Floor lamp" height="62%" primary="Start timer">
      <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 8 }}>
        {[5, 15, 30, 60, 90, 120, 240, "Custom"].map(m => (
          <button key={m} className="card" style={{ padding: "16px 0", textAlign: "center", borderColor: m === 30 ? "var(--on)" : "var(--hairline)", background: m === 30 ? "var(--on-soft)" : "var(--card)" }}>
            <div style={{ fontSize: 18, fontWeight: 600, color: m === 30 ? "var(--on)" : "var(--text)" }}>{m}</div>
            {typeof m === "number" && <div style={{ fontSize: 10.5, color: "var(--text-mute)", marginTop: 2 }}>min</div>}
          </button>
        ))}
      </div>

      <div className="card" style={{ padding: 16, gap: 6, marginTop: 6 }}>
        <div style={{ color: "var(--text-mute)", fontSize: 11.5, fontFamily: "var(--font-mono)", letterSpacing: "0.08em", textTransform: "uppercase" }}>Will fire at</div>
        <div className="num-display" style={{ fontSize: 32 }}>19:42</div>
        <div style={{ color: "var(--text-mute)", fontSize: 12.5 }}>Today, in {minutes} minutes</div>
      </div>
    </Sheet>
  );
}

Object.assign(window, {
  ConfirmModalScreen, ToastsScreen, EmptyStatesScreen, TimerSheet,
});
