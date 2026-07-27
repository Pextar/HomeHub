// Login profiles and their per-user push notification preferences.

/** Per-user push notification preferences. Categories default to true on first subscribe. */
export interface NotifPrefs {
  sensor_alerts: boolean;
  state_changes: boolean;
  schedule_fired: boolean;
  device_offline: boolean;
  // Quiet hours suppress everything except sensor alerts between
  // quiet_start and quiet_end (local time, may wrap past midnight).
  quiet_hours?: boolean;
  quiet_start?: string; // "HH:MM"
  quiet_end?: string;   // "HH:MM"
  // Devices opted out of notifications while their category stays enabled.
  muted_socket_ids?: string[];
  muted_sensor_ids?: string[];
}

// A login profile. Non-admins only see/control the sockets in socket_ids;
// admins ignore that list and have full access.
//
// Roles:
//   - owner=true, admin=true  → the one bootstrapped system owner
//   - owner=false, admin=true → manager (full access, added via invite link)
//   - admin=false             → limited profile (login code, specific devices)
export interface User {
  id: string;
  username: string;
  admin: boolean;
  /** True for the one bootstrapped owner — cannot be deleted or demoted. */
  owner?: boolean;
  /** True while the invite link hasn't been accepted yet (no password set). */
  pending_invite?: boolean;
  // A limited profile rendered with the playful, oversized kid layout.
  kid: boolean;
  // Limited profiles sign in with this generated code instead of a password;
  // empty/absent for admins.
  login_code?: string;
  socket_ids: string[];
  created_at: string;
  notif_prefs?: NotifPrefs;
}

// New admin users get an invite link — no password is set at creation time.
// Limited profiles (admin: false) get a code generated server-side.
export interface UserCreate {
  username: string;
  admin: boolean;
  kid?: boolean;
  socket_ids: string[];
}

// Response from POST /api/users when creating an admin (manager) profile.
// invite_url is only present in this one response; store it before closing.
export interface UserCreateResponse extends User {
  invite_url?: string;
}

// All fields optional — only the ones present are changed. An empty/omitted
// password leaves the existing one untouched. Set regenerate_code to issue a
// fresh login code for a limited profile.
export interface UserUpdate {
  username?: string;
  password?: string;
  admin?: boolean;
  kid?: boolean;
  socket_ids?: string[];
  regenerate_code?: boolean;
}

/** Shape expected by POST /api/push/subscribe */
export interface PushSubscriptionBody {
  endpoint: string;
  keys: { p256dh: string; auth: string };
}
