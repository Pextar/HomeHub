/* HomeHub redesign — App shell + design canvas wiring */

const TWEAK_DEFAULTS = window.TWEAK_DEFAULTS || {
  theme: "dark",
  accent: "#f5bd6e",
};

function hexToRgb(h) {
  const m = /^#?([0-9a-f]{6})$/i.exec(h);
  if (!m) return [245, 189, 110];
  const n = parseInt(m[1], 16);
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}

function applyTheme(theme, accent) {
  document.documentElement.setAttribute("data-theme", theme);
  const [r, g, b] = hexToRgb(accent);
  const s = document.documentElement.style;
  s.setProperty("--on", accent);
  s.setProperty("--on-soft", `rgba(${r},${g},${b},0.14)`);
  s.setProperty("--on-glow", `rgba(${r},${g},${b},0.45)`);
}

// inline wrapper helpers — these RETURN raw element trees rather than
// being React components, so DCSection's `c.type === DCArtboard` filter
// still sees the artboard.
const phone = (id, label, content) => (
  <DCArtboard id={id} label={label} width={402} height={874}>
    <IOSDevice dark={true}>{content}</IOSDevice>
  </DCArtboard>
);
const desk = (id, label, content) => (
  <DCArtboard id={id} label={label} width={1180} height={760}>
    <MacAppWindow title="HomeHub" width={1180} height={760}>
      {content}
    </MacAppWindow>
  </DCArtboard>
);
// raw artboard — no device chrome (for watch, widgets, etc)
const raw = (id, label, w, h, content) => (
  <DCArtboard id={id} label={label} width={w} height={h}>
    {content}
  </DCArtboard>
);

function App() {
  const [t, setTweak] = useTweaks(TWEAK_DEFAULTS);
  React.useEffect(() => { applyTheme(t.theme, t.accent); }, [t.theme, t.accent]);

  return (
    <>
      <DesignCanvas
        title="HomeHub — iPhone + Mac PWA"
        subtitle="Installed PWA on iOS standalone + macOS Add-to-Dock · warm dark · Geist"
      >

        <DCSection id="pwa" title="PWA — install & launch" subtitle="NEW — the surfaces unique to a Progressive Web App on iPhone + Mac">
          {raw  ("icon",     "App icon at scale",        720, 720, <AppIconShowcase/>)}
          {phone("splash",   "iOS launch screen",        <IOSLaunchScreen/>)}
          {phone("a2hs",     "iOS — Add to Home Screen", <AddToHomeScreenSheet/>)}
          {raw  ("a2dock",   "macOS — Add to Dock",       1180, 760, <MacAppWindow title="raspberrypi.local" width={1180} height={760}><AddToDockPrompt/></MacAppWindow>)}
          {phone("offline",  "Offline / hub unreachable", <OfflineHubState/>)}
          {raw  ("menubar",  "macOS menu-bar extra",      560, 720, <MenuBarExtra/>)}
        </DCSection>

        <DCSection id="onboarding" title="Onboarding & Login" subtitle="First-touch surfaces · NEW: invite flow for new users">
          {phone("login-code",  "Login · code mode",                <LoginCodeScreen/>)}
          {phone("login-admin", "Login · admin",                    <LoginAdminScreen/>)}
          {phone("invite",      "NEW — Set password (invite)",      <InviteSetPasswordScreen/>)}
          {phone("kid-home",    "Kid mode home",                    <KidHomeScreen/>)}
        </DCSection>

        <DCSection id="mobile-core" title="iPhone — installed PWA" subtitle="402×874 · standalone mode · the five tabs">
          {phone("home",   "01 · Home",       <HomeScreen/>)}
          {phone("rooms",  "02 · Rooms",      <RoomsScreen/>)}
          {phone("scenes", "03 · Scenes",     <ScenesScreen/>)}
          {phone("sched",  "04 · Schedules",  <SchedulesScreen/>)}
          {phone("set",    "05 · Settings",   <SettingsScreen/>)}
          {phone("console", "05a · Console (Settings → System → Console)", <ConsoleScreen/>)}
        </DCSection>

        <DCSection id="notifications" title="Notifications" subtitle="NEW · Web Push subscription + in-app inbox">
          {phone("push-perm", "Permission prompt", <PushPermissionScreen/>)}
          {phone("inbox",     "Inbox",             <NotificationsInboxScreen/>)}
        </DCSection>

        <DCSection id="mobile-detail" title="Mobile — list & detail views">
          {phone("devices",   "Devices (all)",  <DevicesScreen/>)}
          {phone("groups",    "Groups",         <GroupsScreen/>)}
          {phone("sensors",   "Sensors",        <SensorsScreen/>)}
          {phone("users",     "Users (admin)",  <UsersScreen/>)}
          {phone("floor",     "Floor plan · spatial blueprint", <SpatialBlueprintScreen/>)}
          {phone("light",     "Light detail",   <LightDetailScreen/>)}
        </DCSection>

        <DCSection id="forms" title="Forms & bottom sheets" subtitle="Adding & editing">
          {phone("add-device", "Add device · picker", <AddDeviceScreen/>)}
          {phone("form-sched", "New schedule",        <ScheduleFormScreen/>)}
          {phone("form-scene", "New scene",           <SceneFormScreen/>)}
          {phone("form-group", "Edit group",          <GroupFormScreen/>)}
          {phone("form-sock",  "Edit device",         <SocketFormScreen/>)}
          {phone("form-sens",  "Edit sensor",         <SensorFormScreen/>)}
          {phone("timer",      "One-shot timer",      <TimerSheet/>)}
        </DCSection>

        <DCSection id="matter" title="Matter commissioning flow" subtitle="QR scan → connecting → Wi-Fi → naming">
          {phone("m1", "Step 1 · Scan QR",      <MatterStep1QR/>)}
          {phone("m2", "Step 2 · Connecting",   <MatterStep2Connecting/>)}
          {phone("m3", "Step 3 · Wi-Fi",        <MatterStep3WiFi/>)}
          {phone("m4", "Step 4 · Name device",  <MatterStep4Done/>)}
        </DCSection>

        <DCSection id="states" title="System states" subtitle="Modals, toasts, empty states">
          {phone("confirm", "Confirm dialog", <ConfirmModalScreen/>)}
          {phone("toasts",  "Toast variants", <ToastsScreen/>)}
          {phone("empty",   "Empty states",   <EmptyStatesScreen/>)}
        </DCSection>

        <DCSection id="desktop" title="Mac — installed PWA" subtitle="Runs in its own dark window. No URL bar, no tab strip. Cmd-K to command.">
          {desk("dash",      "Dashboard",             <DesktopDashboard/>)}
          {desk("d-sched",   "Schedules",             <DesktopSchedules/>)}
          {desk("d-sensors", "Sensors",               <DesktopSensors/>)}
          {desk("d-users",   "Users (admin)",         <DesktopUsers/>)}
          {desk("d-notif",   "Notifications popover", <DesktopNotificationsPanel/>)}
          {raw ("d-compact", "Compact window (resized small)", 460, 760, <MacCompactWindow/>)}
        </DCSection>

        <DCSection id="insights" title="Insights & Energy" subtitle="NEW — charts, breakdowns, cost · the missing analytics layer">
          {phone("insights",   "Insights · mobile",  <InsightsScreen/>)}
          {desk ("d-insights", "Insights · desktop", <DesktopInsights/>)}
        </DCSection>

        <DCSection id="automations" title="Automations" subtitle="NEW — WHEN ··· THEN rule engine · list, builder, desktop editor">
          {phone("auto-list",   "Automations list",         <AutomationsScreen/>)}
          {phone("auto-build",  "Builder · step 3 of 4",    <AutomationBuilderScreen/>)}
          {desk ("d-auto",      "Automations · desktop",    <DesktopAutomations/>)}
        </DCSection>

        <DCSection id="activity" title="Activity" subtitle="NEW — timeline of every command and trigger">
          {phone("activity",   "Activity · mobile",   <ActivityScreen/>)}
          {desk ("d-activity", "Activity · desktop",  <DesktopActivity/>)}
        </DCSection>

        <DCSection id="surfaces" title="System surfaces" subtitle="NEW — Lock screen widgets · Apple Watch">
          {phone("lock",       "iOS Lock Screen · widgets", <IOSLockScreen/>)}
          {raw("w-home",   "Watch · Home",   320, 380, <WatchHome/>)}
          {raw("w-scenes", "Watch · Scenes", 320, 380, <WatchScenes/>)}
          {raw("w-light",  "Watch · Light",  320, 380, <WatchLight/>)}
        </DCSection>
      </DesignCanvas>

      <TweaksPanel>
        <TweakSection title="Theme">
          <TweakRadio
            label="Mode"
            value={t.theme}
            options={["dark", "light"]}
            onChange={v => setTweak("theme", v)}
          />
        </TweakSection>
        <TweakSection title="Accent">
          <TweakColor
            label="ON color"
            value={t.accent}
            options={["#f5bd6e", "#7aa4d9", "#8fcfa8", "#e8a09a"]}
            onChange={v => setTweak("accent", v)}
          />
        </TweakSection>
      </TweaksPanel>
    </>
  );
}

ReactDOM.createRoot(document.getElementById("root")).render(<App/>);
