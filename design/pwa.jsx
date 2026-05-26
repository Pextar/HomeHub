/* HomeHub — PWA-focused surfaces for iPhone + Mac.

   Adds:
   · MacAppWindow         dark macOS-native window chrome (for installed PWA)
   · AppIconShowcase      app icon at multiple sizes
   · IOSLaunchScreen      splash shown during PWA cold start
   · AddToHomeScreenSheet iOS Safari Share-sheet flow
   · AddToDockPrompt      macOS Safari "Add to Dock" prompt
   · OfflineHubState      hub unreachable empty/error
   · StandaloneHomeScreen iPhone home tuned for PWA standalone (safe area, no Safari bar)
   · MacPWADashboard      compact installed-Mac window dashboard
   · MacPWAInsights       compact installed-Mac window insights
*/

const PAD3 = 22;

// ─────────────────────────────────────────────────────────────
// MacAppWindow — dark titlebar w/ traffic lights, blends into app
// ─────────────────────────────────────────────────────────────
function MacAppWindow({ title = "HomeHub", subtitle, width = 1180, height = 760, children }) {
  return (
    <div style={{
      width, height,
      borderRadius: 12,
      overflow: "hidden",
      background: "var(--bg)",
      boxShadow: "0 0 0 1px rgba(0,0,0,0.5), 0 30px 80px rgba(0,0,0,0.55)",
      display: "flex", flexDirection: "column",
      fontFamily: "var(--font-sans)",
      position: "relative",
    }}>
      {/* titlebar */}
      <div style={{
        height: 40,
        background: "linear-gradient(to bottom, #1a1812, #14130f)",
        borderBottom: "1px solid #000",
        display: "grid",
        gridTemplateColumns: "120px 1fr 120px",
        alignItems: "center",
        padding: "0 12px",
        flexShrink: 0,
        position: "relative",
        zIndex: 2,
      }}>
        <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
          <div style={{ width: 12, height: 12, borderRadius: "50%", background: "#ff5f57", border: "0.5px solid rgba(0,0,0,0.2)" }}/>
          <div style={{ width: 12, height: 12, borderRadius: "50%", background: "#febc2e", border: "0.5px solid rgba(0,0,0,0.2)" }}/>
          <div style={{ width: 12, height: 12, borderRadius: "50%", background: "#28c840", border: "0.5px solid rgba(0,0,0,0.2)" }}/>
        </div>
        <div style={{ textAlign: "center", fontSize: 13, fontWeight: 500, color: "var(--text-mute)", letterSpacing: "-0.005em" }}>
          {title}
          {subtitle && <span style={{ color: "var(--text-dim)", marginLeft: 8 }}>— {subtitle}</span>}
        </div>
        <div/>
      </div>
      <div style={{ flex: 1, overflow: "hidden", background: "var(--bg)" }}>
        {children}
      </div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// HomeHub mark — used in icons, splash, etc
// ─────────────────────────────────────────────────────────────
const HomeHubMark = ({ size = 200 }) => {
  const r = size * 0.22;
  const inner = size * 0.34;
  return (
    <div style={{
      width: size, height: size,
      borderRadius: r,
      background: "linear-gradient(150deg, #f5bd6e 0%, #d99a4c 55%, #b27a32 100%)",
      display: "grid", placeItems: "center",
      position: "relative",
      boxShadow: `inset 0 ${size*0.01}px 0 rgba(255,255,255,0.4), 0 ${size*0.04}px ${size*0.08}px rgba(0,0,0,0.4)`,
    }}>
      <div style={{
        width: inner, height: inner,
        borderRadius: size * 0.07,
        background: "#1a1408",
        boxShadow: `inset 0 0 ${size * 0.03}px rgba(0,0,0,0.6)`,
      }}/>
    </div>
  );
};

// ─────────────────────────────────────────────────────────────
// AppIconShowcase — icon at multiple sizes
// ─────────────────────────────────────────────────────────────
function AppIconShowcase() {
  return (
    <div style={{
      width: "100%", height: "100%",
      background: "linear-gradient(140deg, #1a1812 0%, #0c0b09 100%)",
      padding: 48,
      display: "flex", flexDirection: "column", gap: 36,
      color: "var(--text)", fontFamily: "var(--font-sans)",
      overflow: "hidden",
    }}>
      <div>
        <div style={{ color: "var(--text-mute)", fontSize: 12, fontFamily: "var(--font-mono)", letterSpacing: "0.12em", textTransform: "uppercase" }}>
          App icon · iOS / iPadOS / macOS
        </div>
        <h1 style={{ fontSize: 32, fontWeight: 600, letterSpacing: "-0.02em", marginTop: 6 }}>HomeHub mark</h1>
        <div style={{ color: "var(--text-mute)", fontSize: 14, marginTop: 4 }}>
          Hub-and-spoke metaphor — incandescent shell, deep-warm core.
        </div>
      </div>

      <div style={{ display: "flex", alignItems: "flex-end", gap: 32 }}>
        <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 10 }}>
          <HomeHubMark size={180}/>
          <div className="mono" style={{ fontSize: 11, color: "var(--text-mute)" }}>1024 · 180 · 120</div>
          <div style={{ fontSize: 11.5, color: "var(--text-dim)" }}>App Store · Home Screen</div>
        </div>
        <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 10 }}>
          <HomeHubMark size={120}/>
          <div className="mono" style={{ fontSize: 11, color: "var(--text-mute)" }}>120</div>
          <div style={{ fontSize: 11.5, color: "var(--text-dim)" }}>Home Screen</div>
        </div>
        <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 10 }}>
          <HomeHubMark size={76}/>
          <div className="mono" style={{ fontSize: 11, color: "var(--text-mute)" }}>76</div>
          <div style={{ fontSize: 11.5, color: "var(--text-dim)" }}>Spotlight</div>
        </div>
        <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 10 }}>
          <HomeHubMark size={48}/>
          <div className="mono" style={{ fontSize: 11, color: "var(--text-mute)" }}>48</div>
          <div style={{ fontSize: 11.5, color: "var(--text-dim)" }}>Mac dock</div>
        </div>
        <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 10 }}>
          <HomeHubMark size={28}/>
          <div className="mono" style={{ fontSize: 11, color: "var(--text-mute)" }}>28</div>
          <div style={{ fontSize: 11.5, color: "var(--text-dim)" }}>Notifications</div>
        </div>
      </div>

      {/* spec strip */}
      <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 14, marginTop: "auto" }}>
        {[
          { l: "Shell",     v: "#F5BD6E", swatch: "#F5BD6E" },
          { l: "Core",      v: "#1A1408", swatch: "#1A1408" },
          { l: "Corner",    v: "22% · iOS squircle", swatch: null },
          { l: "Format",    v: "PNG · transparent", swatch: null },
        ].map(s => (
          <div key={s.l} className="card" style={{ padding: 14 }}>
            <div style={{ color: "var(--text-mute)", fontSize: 10.5, fontFamily: "var(--font-mono)", letterSpacing: "0.1em", textTransform: "uppercase" }}>{s.l}</div>
            <div style={{ display: "flex", alignItems: "center", gap: 8, marginTop: 6 }}>
              {s.swatch && <div style={{ width: 14, height: 14, borderRadius: 4, background: s.swatch, border: "1px solid var(--hairline)" }}/>}
              <span className="mono" style={{ fontSize: 13, color: "var(--text)" }}>{s.v}</span>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// IOSLaunchScreen — splash during PWA cold start
// ─────────────────────────────────────────────────────────────
function IOSLaunchScreen() {
  return (
    <div className="hh" style={{
      position: "relative", height: "100%", overflow: "hidden",
      background: "radial-gradient(ellipse at 50% 35%, #2a2218 0%, #14130f 60%, #0a0a08 100%)",
      display: "flex", flexDirection: "column",
    }}>
      <div style={{ height: 54 }}/>
      <div style={{ flex: 1, display: "grid", placeItems: "center" }}>
        <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 22 }}>
          <div style={{ position: "relative" }}>
            <HomeHubMark size={104}/>
            <div style={{ position: "absolute", inset: -16, borderRadius: "50%", background: "radial-gradient(closest-side, rgba(245,189,110,0.30), transparent 70%)", zIndex: -1 }}/>
          </div>
          <div style={{ textAlign: "center" }}>
            <div style={{ fontSize: 28, fontWeight: 600, letterSpacing: "-0.02em" }}>HomeHub</div>
            <div style={{ color: "var(--text-mute)", fontSize: 13, marginTop: 4, fontFamily: "var(--font-mono)" }}>raspberrypi.local</div>
          </div>
        </div>
      </div>
      <div style={{ paddingBottom: 60, display: "flex", flexDirection: "column", alignItems: "center", gap: 10 }}>
        <div style={{ width: 28, height: 28, borderRadius: "50%", border: "2px solid var(--card-3)", borderTopColor: "var(--on)", animation: "none" }}/>
        <div style={{ color: "var(--text-dim)", fontSize: 11, fontFamily: "var(--font-mono)", letterSpacing: "0.06em" }}>Reconnecting to hub…</div>
      </div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// AddToHomeScreenSheet — iOS Safari share-sheet instructions
// ─────────────────────────────────────────────────────────────
function AddToHomeScreenSheet() {
  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "hidden", background: "rgba(0,0,0,0.4)" }}>
      <div style={{ height: 54 }}/>
      {/* faded webpage in bg */}
      <div style={{ padding: `8px ${PAD3}px 0`, opacity: 0.4 }}>
        <div style={{ color: "var(--text-mute)", fontSize: 13 }}>Tuesday, 9:41</div>
        <h1 style={{ fontSize: 30, fontWeight: 600, marginTop: 4 }}>Good evening,<br/><span style={{ color: "var(--text-mute)" }}>Mira</span></h1>
      </div>

      {/* dim overlay */}
      <div style={{ position: "absolute", inset: 0, background: "rgba(0,0,0,0.55)" }}/>

      {/* sheet */}
      <div style={{
        position: "absolute", left: 12, right: 12, bottom: 24,
        background: "var(--card)",
        borderRadius: 22,
        border: "1px solid var(--hairline)",
        padding: 22,
        boxShadow: "0 24px 60px rgba(0,0,0,0.6)",
      }}>
        <div style={{ display: "flex", alignItems: "center", gap: 14, marginBottom: 16 }}>
          <HomeHubMark size={56}/>
          <div style={{ flex: 1 }}>
            <div style={{ fontSize: 16, fontWeight: 600 }}>Install HomeHub</div>
            <div style={{ color: "var(--text-mute)", fontSize: 12.5, marginTop: 2 }}>Add to Home Screen for full-screen mode, faster launch, and push notifications.</div>
          </div>
        </div>

        {/* steps */}
        <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          {[
            { n: "1", title: "Tap the Share button", det: "in Safari's bottom toolbar", glyph: "share" },
            { n: "2", title: "Choose “Add to Home Screen”", det: "scroll down in the sheet", glyph: "plus" },
            { n: "3", title: "Tap “Add”", det: "HomeHub appears on your Home Screen", glyph: "home" },
          ].map(s => (
            <div key={s.n} style={{ display: "flex", alignItems: "center", gap: 12, padding: "10px 12px", background: "var(--card-2)", borderRadius: 12 }}>
              <div style={{ width: 26, height: 26, borderRadius: "50%", background: "var(--on)", color: "#1a1813", display: "grid", placeItems: "center", fontWeight: 700, fontSize: 13, fontFamily: "var(--font-mono)", flexShrink: 0 }}>{s.n}</div>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontSize: 13.5, fontWeight: 500 }}>{s.title}</div>
                <div style={{ color: "var(--text-mute)", fontSize: 11.5 }}>{s.det}</div>
              </div>
              {/* glyph */}
              <div style={{ width: 30, height: 30, borderRadius: 8, background: "var(--card-3)", display: "grid", placeItems: "center", color: "var(--on)", flexShrink: 0 }}>
                {s.glyph === "share" && (
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M12 3v13M8 7l4-4 4 4M5 12v7a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-7"/>
                  </svg>
                )}
                {s.glyph === "plus" && <Icon d={I.plus} size={14} stroke={2}/>}
                {s.glyph === "home" && <Icon d={I.home} size={14} stroke={1.7}/>}
              </div>
            </div>
          ))}
        </div>

        <button style={{
          width: "100%", marginTop: 16, padding: "14px",
          background: "var(--card-3)", color: "var(--text)",
          borderRadius: 12, fontSize: 14, fontWeight: 500,
        }}>Not now</button>
      </div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// OfflineHubState — hub unreachable error screen
// ─────────────────────────────────────────────────────────────
function OfflineHubState() {
  return (
    <div className="hh" style={{ position: "relative", height: "100%", overflow: "hidden", display: "flex", flexDirection: "column" }}>
      <div style={{ height: 54 }}/>

      {/* top inline alert */}
      <div style={{ margin: `0 ${PAD3}px`, padding: "12px 14px", background: "rgba(224,138,122,0.12)", border: "1px solid rgba(224,138,122,0.25)", borderRadius: 12, display: "flex", alignItems: "center", gap: 10 }}>
        <div style={{ width: 8, height: 8, borderRadius: "50%", background: "var(--bad)", boxShadow: "0 0 0 4px rgba(224,138,122,0.18)" }}/>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ fontSize: 13, fontWeight: 500 }}>Hub unreachable</div>
          <div style={{ color: "var(--text-mute)", fontSize: 11.5, marginTop: 1 }}>Last seen <span className="mono">2 min ago</span> · still on local network?</div>
        </div>
        <button style={{ fontSize: 12, color: "var(--on)", fontWeight: 500, padding: "6px 10px", background: "var(--on-soft)", borderRadius: 8 }}>Retry</button>
      </div>

      {/* center illustration */}
      <div style={{ flex: 1, display: "grid", placeItems: "center", padding: PAD3 }}>
        <div style={{ textAlign: "center", maxWidth: 320 }}>
          {/* concentric rings — bad signal */}
          <div style={{ position: "relative", width: 140, height: 140, margin: "0 auto 22px" }}>
            <div style={{ position: "absolute", inset: 0, borderRadius: "50%", border: "1px dashed var(--border)" }}/>
            <div style={{ position: "absolute", inset: 22, borderRadius: "50%", border: "1px dashed var(--border)" }}/>
            <div style={{ position: "absolute", inset: 44, borderRadius: "50%", border: "1px dashed var(--border)" }}/>
            <div style={{ position: "absolute", inset: 0, display: "grid", placeItems: "center" }}>
              <div style={{ width: 50, height: 50, borderRadius: 14, background: "var(--card-2)", border: "1px solid var(--border)", display: "grid", placeItems: "center", color: "var(--bad)" }}>
                <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round">
                  <rect x="6" y="3" width="12" height="18" rx="2"/>
                  <path d="M9 8h6M9 12h6M9 16h3"/>
                </svg>
              </div>
            </div>
            {/* slash */}
            <svg viewBox="0 0 140 140" style={{ position: "absolute", inset: 0 }} stroke="var(--bad)" strokeWidth="2" strokeLinecap="round">
              <line x1="30" y1="30" x2="110" y2="110"/>
            </svg>
          </div>

          <h2 style={{ fontSize: 22, fontWeight: 600, letterSpacing: "-0.02em" }}>Can't reach your hub</h2>
          <div style={{ color: "var(--text-mute)", fontSize: 13.5, marginTop: 8, lineHeight: 1.5 }}>
            HomeHub is offline. Your Pi may have lost power, dropped Wi-Fi, or your phone left the network.
          </div>

          <div className="card" style={{ marginTop: 22, textAlign: "left" }}>
            {[
              { l: "Check power LED on Pi",   sub: "Green = OK · Red = no power" },
              { l: "Re-join your Wi-Fi",      sub: "Currently on: <span class='mono'>—</span>" },
              { l: "Open Tailscale (remote)", sub: "Connect from outside the LAN" },
            ].map((c, i, a) => (
              <div key={c.l} style={{ padding: "12px 16px", borderBottom: i < a.length-1 ? "1px solid var(--hairline)" : "none", display: "flex", alignItems: "center", gap: 12 }}>
                <div style={{ width: 8, height: 8, borderRadius: "50%", background: "var(--text-dim)" }}/>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ fontSize: 13.5, fontWeight: 500 }}>{c.l}</div>
                  <div style={{ color: "var(--text-mute)", fontSize: 11.5, marginTop: 2 }} dangerouslySetInnerHTML={{ __html: c.sub }}/>
                </div>
                <Icon d={I.chevR} size={14} stroke={2} style={{ color: "var(--text-dim)" }}/>
              </div>
            ))}
          </div>

          <button style={{ marginTop: 18, padding: "14px 22px", background: "var(--on)", color: "#1a1813", borderRadius: 14, fontWeight: 600, fontSize: 14 }}>
            Try again
          </button>
        </div>
      </div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// AddToDockPrompt — macOS Safari "Add to Dock" prompt (inside MacAppWindow scope)
// ─────────────────────────────────────────────────────────────
function AddToDockPrompt() {
  return (
    <div style={{
      width: "100%", height: "100%",
      background: "radial-gradient(ellipse at 50% 0%, #2a2218 0%, var(--bg) 60%)",
      display: "flex", padding: 36, gap: 36,
      color: "var(--text)", fontFamily: "var(--font-sans)", overflow: "hidden",
    }}>
      {/* left — explanation */}
      <div style={{ flex: 1, display: "flex", flexDirection: "column", justifyContent: "center", maxWidth: 480 }}>
        <div style={{ color: "var(--on)", fontSize: 12, fontFamily: "var(--font-mono)", letterSpacing: "0.12em", textTransform: "uppercase" }}>
          Install on Mac
        </div>
        <h1 style={{ fontSize: 44, fontWeight: 600, letterSpacing: "-0.025em", lineHeight: 1.1, marginTop: 10 }}>
          Add HomeHub<br/>to your Dock.
        </h1>
        <div style={{ color: "var(--text-mute)", fontSize: 15, marginTop: 14, lineHeight: 1.55 }}>
          Runs in its own window. No tab clutter, no URL bar. Cmd-Tab to switch, Cmd-K to command, and stays signed in to your hub.
        </div>

        <div style={{ marginTop: 26, display: "flex", flexDirection: "column", gap: 12 }}>
          {[
            { n: "1", t: "In Safari, open the File menu", k: "File" },
            { n: "2", t: "Choose “Add to Dock…”",          k: "Add to Dock…" },
            { n: "3", t: "Confirm — done.",                 k: "Add" },
          ].map(s => (
            <div key={s.n} style={{ display: "flex", alignItems: "center", gap: 14 }}>
              <div style={{ width: 26, height: 26, borderRadius: "50%", background: "var(--on)", color: "#1a1813", display: "grid", placeItems: "center", fontWeight: 700, fontSize: 13, fontFamily: "var(--font-mono)", flexShrink: 0 }}>{s.n}</div>
              <span style={{ fontSize: 14, color: "var(--text)" }}>{s.t}</span>
              <span style={{ marginLeft: "auto", padding: "4px 10px", background: "var(--card-2)", border: "1px solid var(--hairline)", borderRadius: 8, fontFamily: "var(--font-mono)", fontSize: 11.5, color: "var(--text-mute)" }}>{s.k}</span>
            </div>
          ))}
        </div>

        <div style={{ marginTop: 30, display: "flex", gap: 10 }}>
          <button style={{ padding: "12px 22px", background: "var(--on)", color: "#1a1813", borderRadius: 10, fontWeight: 600, fontSize: 14 }}>Show me how</button>
          <button style={{ padding: "12px 22px", background: "var(--card)", color: "var(--text)", border: "1px solid var(--border)", borderRadius: 10, fontWeight: 500, fontSize: 14 }}>Maybe later</button>
        </div>
      </div>

      {/* right — preview: Safari menu open showing "Add to Dock" highlighted */}
      <div style={{ flex: 1, display: "grid", placeItems: "center" }}>
        <div style={{ position: "relative" }}>
          {/* fake safari menu */}
          <div style={{
            width: 280,
            background: "rgba(50,50,50,0.86)",
            backdropFilter: "blur(40px) saturate(180%)",
            border: "0.5px solid rgba(255,255,255,0.15)",
            borderRadius: 10,
            padding: "6px 0",
            boxShadow: "0 24px 60px rgba(0,0,0,0.5)",
            fontFamily: "-apple-system, BlinkMacSystemFont, 'SF Pro', sans-serif",
            color: "#fff",
          }}>
            {[
              "New Window",
              "New Tab",
              "New Private Window",
              "Open File…",
              "Open Location…",
              "—",
              "Close Window",
              "Close Tab",
              "Save As…",
              "Share",
              "—",
              { l: "Add to Dock…", active: true },
              "Add Bookmark…",
              "—",
              "Print…",
            ].map((item, i) => {
              if (item === "—") {
                return <div key={i} style={{ height: 1, background: "rgba(255,255,255,0.1)", margin: "4px 0" }}/>;
              }
              const active = typeof item === "object" && item.active;
              const label = typeof item === "object" ? item.l : item;
              return (
                <div key={i} style={{
                  padding: "4px 14px",
                  fontSize: 13,
                  background: active ? "#1a6efe" : "transparent",
                  color: active ? "#fff" : "#fff",
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "center",
                }}>
                  <span style={{ fontWeight: active ? 500 : 400 }}>{label}</span>
                  {active && <span style={{ fontSize: 11, opacity: 0.7 }}>⌘⇧A</span>}
                </div>
              );
            })}
          </div>
          <div style={{ marginTop: 18, textAlign: "center", color: "var(--text-mute)", fontSize: 12, fontFamily: "var(--font-mono)" }}>
            Safari · File menu
          </div>
        </div>
      </div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// MacPWADashboard — compact Mac PWA window dashboard
// (reuses DesktopDashboard but inside MacAppWindow, sized for PWA)
// ─────────────────────────────────────────────────────────────
function MacPWADashboard() {
  return (
    <MacAppWindow title="HomeHub" subtitle="Dashboard" width={1180} height={760}>
      <DesktopDashboard/>
    </MacAppWindow>
  );
}

function MacPWAInsights() {
  return (
    <MacAppWindow title="HomeHub" subtitle="Insights" width={1180} height={760}>
      <DesktopInsights/>
    </MacAppWindow>
  );
}

function MacPWAAutomations() {
  return (
    <MacAppWindow title="HomeHub" subtitle="Automations" width={1180} height={760}>
      <DesktopAutomations/>
    </MacAppWindow>
  );
}

// ─────────────────────────────────────────────────────────────
// MacCompactWindow — a narrow Mac PWA window (sidebar collapses)
// Shows that the app responds well when user resizes small
// ─────────────────────────────────────────────────────────────
function MacCompactWindow() {
  // Use the mobile HomeScreen content but in a Mac frame, no iOS chrome
  return (
    <MacAppWindow title="HomeHub" width={460} height={760}>
      <div className="hh" style={{ height: "100%", overflow: "auto", paddingBottom: 24 }}>
        <div style={{ padding: `18px ${PAD3}px 0`, display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}>
          <div>
            <div style={{ color: "var(--text-mute)", fontSize: 12.5, fontWeight: 500 }}>Tuesday, 9:41</div>
            <h1 style={{ fontSize: 26, fontWeight: 600, marginTop: 4, letterSpacing: "-0.02em" }}>Good evening, Mira</h1>
          </div>
          <button className="chip" style={{ width: 32, height: 32, padding: 0, justifyContent: "center" }}>
            <Icon d={I.search} size={14} stroke={1.7}/>
          </button>
        </div>

        {/* hero */}
        <div style={{ padding: `16px ${PAD3}px 0` }}>
          <div className="tile on" style={{ padding: 18, gap: 12 }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}>
              <div>
                <div style={{ color: "var(--on)", fontSize: 10.5, fontFamily: "var(--font-mono)", letterSpacing: "0.1em", textTransform: "uppercase" }}>Whole home</div>
                <div style={{ display: "flex", alignItems: "baseline", gap: 8, marginTop: 6 }}>
                  <span className="num-display" style={{ fontSize: 44 }}>7</span>
                  <span style={{ color: "var(--text-mute)", fontSize: 13 }}>of 23 on</span>
                </div>
              </div>
              <div className="sw-big on"/>
            </div>
            <div style={{ display: "flex", alignItems: "center", color: "var(--text-mute)", fontSize: 11.5, gap: 8 }}>
              <Icon d={I.energy} size={12} stroke={2} style={{ color: "var(--on)" }}/>
              <span className="mono" style={{ color: "var(--text)" }}>184 W</span> now · <span className="mono" style={{ color: "var(--text)" }}>3.2 kWh</span> today
            </div>
          </div>
        </div>

        {/* favorites grid 2-col */}
        <div style={{ padding: `18px ${PAD3}px 0` }}>
          <div style={{ color: "var(--text-mute)", fontSize: 11, fontFamily: "var(--font-mono)", letterSpacing: "0.1em", textTransform: "uppercase", marginBottom: 10 }}>Favorites</div>
          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 10 }}>
            {[
              { n: "Floor lamp", r: "Living", on: true, dim: 62 },
              { n: "TV strip",   r: "Living", on: true, dim: 28 },
              { n: "Kitchen",    r: "Kitchen", on: true, dim: 100 },
              { n: "Coffee bar", r: "Kitchen", on: false },
            ].map(d => (
              <div key={d.n} className={`tile ${d.on ? "on" : ""}`} style={{ padding: 14 }}>
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}>
                  <div className="tile-bulb">
                    <Icon d={I.bulb} size={16} stroke={1.7} style={{ color: d.on ? "#3a2400" : "var(--text-mute)" }}/>
                  </div>
                  <div className={`sw ${d.on ? "on" : ""}`}/>
                </div>
                <div style={{ marginTop: 8 }}>
                  <div style={{ fontWeight: 600, fontSize: 13.5 }}>{d.n}</div>
                  <div style={{ color: "var(--text-mute)", fontSize: 11.5 }}>{d.on ? (d.dim ? `On · ${d.dim}%` : "On") : "Off"} · {d.r}</div>
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* command bar hint */}
        <div style={{ padding: `18px ${PAD3}px 0` }}>
          <div className="card" style={{ padding: "10px 14px", display: "flex", alignItems: "center", gap: 10, color: "var(--text-mute)", fontSize: 12.5 }}>
            <Icon d={I.search} size={13} stroke={1.7}/>
            <span>Search devices, scenes…</span>
            <span style={{ marginLeft: "auto", fontFamily: "var(--font-mono)", fontSize: 10.5, color: "var(--text-dim)", padding: "2px 6px", background: "var(--card-3)", borderRadius: 5 }}>⌘K</span>
          </div>
        </div>
      </div>
    </MacAppWindow>
  );
}

// ─────────────────────────────────────────────────────────────
// MenuBarExtra — macOS menu-bar dropdown (the PWA's tiny surface)
// ─────────────────────────────────────────────────────────────
function MenuBarExtra() {
  return (
    <div style={{
      width: "100%", height: "100%",
      background: "linear-gradient(180deg, #6a8baa 0%, #4a6a8a 50%, #3a5a7a 100%)",
      padding: "60px 40px",
      display: "flex", justifyContent: "center", alignItems: "flex-start",
      fontFamily: "-apple-system, BlinkMacSystemFont, 'SF Pro', sans-serif",
    }}>
      {/* a fake menu bar slice */}
      <div style={{ position: "relative", width: 320 }}>
        {/* indicator triangle */}
        <div style={{ position: "absolute", top: -16, right: 32, fontSize: 14, color: "var(--on)" }}>
          <div style={{ width: 16, height: 16, background: "var(--on)", borderRadius: 4, display: "grid", placeItems: "center" }}>
            <div style={{ width: 7, height: 7, borderRadius: 1.5, background: "#1a1408" }}/>
          </div>
        </div>

        {/* dropdown */}
        <div style={{
          background: "rgba(28,26,21,0.86)",
          backdropFilter: "blur(40px) saturate(180%)",
          border: "0.5px solid rgba(255,255,255,0.1)",
          borderRadius: 12,
          padding: 12,
          boxShadow: "0 24px 60px rgba(0,0,0,0.4)",
          color: "var(--text)",
          fontFamily: "var(--font-sans)",
        }}>
          {/* header */}
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "4px 6px 10px" }}>
            <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
              <span className="dot on"/>
              <span style={{ fontSize: 12, fontWeight: 600 }}>HomeHub</span>
              <span style={{ color: "var(--text-dim)", fontSize: 11, fontFamily: "var(--font-mono)" }}>· connected</span>
            </div>
            <div style={{ fontSize: 11, color: "var(--on)", fontFamily: "var(--font-mono)" }}>184 W</div>
          </div>

          {/* whole home toggle */}
          <div style={{ background: "linear-gradient(155deg, #2b2419, #221d14)", border: "1px solid rgba(245,189,110,0.15)", borderRadius: 10, padding: 12, display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 10 }}>
            <div>
              <div style={{ fontSize: 10, color: "var(--on)", fontFamily: "var(--font-mono)", letterSpacing: "0.08em", textTransform: "uppercase" }}>Whole home</div>
              <div style={{ display: "flex", alignItems: "baseline", gap: 6, marginTop: 4 }}>
                <span className="num-display" style={{ fontSize: 28 }}>7</span>
                <span style={{ color: "var(--text-mute)", fontSize: 11 }}>of 23 on</span>
              </div>
            </div>
            <div className="sw on"/>
          </div>

          {/* scene quick chips */}
          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 6, marginBottom: 10 }}>
            {[
              { n: "Evening",   active: true,  c: "var(--on)" },
              { n: "Goodnight", c: "var(--cool)" },
              { n: "Movie",     c: "#a96bd9" },
            ].map(s => (
              <div key={s.n} style={{
                padding: "8px 6px",
                borderRadius: 8,
                background: s.active ? "var(--on-soft)" : "var(--card-2)",
                border: s.active ? "1px solid var(--on)" : "1px solid transparent",
                textAlign: "center",
              }}>
                <div style={{ width: 12, height: 12, borderRadius: "50%", background: s.c, opacity: s.active ? 1 : 0.6, margin: "0 auto 4px" }}/>
                <div style={{ fontSize: 10.5, fontWeight: 500 }}>{s.n}</div>
              </div>
            ))}
          </div>

          {/* device rows */}
          <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
            {[
              { n: "Floor lamp", r: "Living", on: true,  v: "62%" },
              { n: "TV strip",   r: "Living", on: true,  v: "28%" },
              { n: "Kitchen",    r: "Kitchen",on: true,  v: "100%" },
              { n: "Bedroom",    r: "Bed",    on: false, v: "off" },
            ].map(d => (
              <div key={d.n} style={{ display: "flex", alignItems: "center", gap: 10, padding: "8px 6px", borderRadius: 6 }}>
                <div className={`dot ${d.on ? "on" : ""}`} style={{ background: d.on ? "var(--on)" : "var(--text-dim)", boxShadow: d.on ? "0 0 0 3px var(--on-soft)" : "none" }}/>
                <div style={{ flex: 1, fontSize: 12 }}>{d.n}</div>
                <span style={{ fontFamily: "var(--font-mono)", fontSize: 10.5, color: d.on ? "var(--on)" : "var(--text-dim)" }}>{d.v}</span>
              </div>
            ))}
          </div>

          {/* footer */}
          <div style={{ marginTop: 8, padding: "8px 6px 4px", borderTop: "1px solid var(--hairline)", display: "flex", justifyContent: "space-between", alignItems: "center" }}>
            <span style={{ fontSize: 11, color: "var(--text-mute)" }}>Open HomeHub</span>
            <span style={{ fontFamily: "var(--font-mono)", fontSize: 10, color: "var(--text-dim)" }}>⌘⇧H</span>
          </div>
        </div>
      </div>
    </div>
  );
}

// expose
Object.assign(window, {
  MacAppWindow,
  HomeHubMark,
  AppIconShowcase,
  IOSLaunchScreen,
  AddToHomeScreenSheet,
  OfflineHubState,
  AddToDockPrompt,
  MacPWADashboard,
  MacPWAInsights,
  MacPWAAutomations,
  MacCompactWindow,
  MenuBarExtra,
});
