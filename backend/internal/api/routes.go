package api

import (
	"github.com/gorilla/mux"

	"homehub/internal/kef"
	"homehub/internal/media"
	"homehub/internal/sonos"
)

// registerRoutes mounts every API route onto the authenticated /api subrouter.
//
// Registration order is significant: gorilla/mux matches in the order routes
// are added, so literal paths must be registered before the {id} patterns that
// would otherwise swallow them. Each group below preserves the order it had
// when this table lived inline in Handler(); keep new routes inside the group
// they belong to rather than appending at the end.
func (s *Server) registerRoutes(api *mux.Router) {
	s.registerCoreRoutes(api)
	s.registerUserRoutes(api)
	s.registerSocketRoutes(api)
	s.registerRoomRoutes(api)
	s.registerScheduleRoutes(api)
	s.registerAutomationRoutes(api)
	s.registerGroupRoutes(api)
	s.registerSceneRoutes(api)
	s.registerTimerRoutes(api)
	s.registerSensorRoutes(api)
	s.registerAdminRoutes(api)
	s.registerTasmotaRoutes(api)
	s.registerSonosRoutes(api)
	s.registerKEFRoutes(api)
	s.registerMediaRoutes(api)
	s.registerSpotifyRoutes(api)
	s.registerMatterRoutes(api)
	s.registerMQTTRoutes(api)
	s.registerAssistantRoutes(api)
}

func (s *Server) registerCoreRoutes(api *mux.Router) {
	api.HandleFunc("/health", s.getHealth).Methods("GET")
	api.HandleFunc("/events", s.handleEvents).Methods("GET")
}

func (s *Server) registerUserRoutes(api *mux.Router) {
	api.HandleFunc("/me", s.getMe).Methods("GET")
	api.HandleFunc("/users", s.requireAdmin(s.listUsers)).Methods("GET")
	api.HandleFunc("/users", s.requireAdmin(s.createUser)).Methods("POST")
	api.HandleFunc("/users/{id}", s.requireAdmin(s.updateUser)).Methods("PUT")
	api.HandleFunc("/users/{id}", s.requireAdmin(s.deleteUser)).Methods("DELETE")
}

// registerSocketRoutes mounts the socket surface. Lists are filtered to the
// caller's allowed set, control endpoints are gated per-socket, and
// create/edit/delete are admin-only.
func (s *Server) registerSocketRoutes(api *mux.Router) {
	api.HandleFunc("/sockets", s.getSockets).Methods("GET")
	api.HandleFunc("/sockets", s.requireAdmin(s.createSocket)).Methods("POST")
	api.HandleFunc("/sockets/learn", s.requireAdmin(s.learnSocket)).Methods("POST")
	api.HandleFunc("/sockets/all/on", s.bulkSetState(true)).Methods("POST")
	api.HandleFunc("/sockets/all/off", s.bulkSetState(false)).Methods("POST")
	api.HandleFunc("/sockets/{id}", s.getSocket).Methods("GET")
	api.HandleFunc("/sockets/{id}", s.requireAdmin(s.updateSocket)).Methods("PUT")
	api.HandleFunc("/sockets/{id}", s.requireAdmin(s.deleteSocket)).Methods("DELETE")
	api.HandleFunc("/sockets/{id}/toggle", s.toggleSocket).Methods("POST")
	api.HandleFunc("/sockets/{id}/on", s.turnOn).Methods("POST")
	api.HandleFunc("/sockets/{id}/off", s.turnOff).Methods("POST")
	api.HandleFunc("/sockets/{id}/timer", s.createSocketTimer).Methods("POST")
	api.HandleFunc("/sockets/{id}/favorite", s.toggleFavorite).Methods("POST")
}

func (s *Server) registerRoomRoutes(api *mux.Router) {
	api.HandleFunc("/rooms", s.getRooms).Methods("GET")
	api.HandleFunc("/rooms", s.requireAdmin(s.createRoom)).Methods("POST")
	api.HandleFunc("/rooms/{id}", s.requireAdmin(s.updateRoom)).Methods("PUT")
	api.HandleFunc("/rooms/{id}", s.requireAdmin(s.deleteRoom)).Methods("DELETE")
	api.HandleFunc("/rooms/{room}/on", s.roomSetState(true)).Methods("POST")
	api.HandleFunc("/rooms/{room}/off", s.roomSetState(false)).Methods("POST")
}

// registerScheduleRoutes mounts schedule read/write, which is open to all
// authenticated users; handlers filter results to the caller's own sockets for
// non-admins. The bulk enable/disable ("vacation mode") remains admin-only.
func (s *Server) registerScheduleRoutes(api *mux.Router) {
	api.HandleFunc("/schedules", s.getSchedules).Methods("GET")
	api.HandleFunc("/schedules", s.createSchedule).Methods("POST")
	api.HandleFunc("/schedules/all/enable", s.requireAdmin(s.setAllSchedules(true))).Methods("POST")
	api.HandleFunc("/schedules/all/disable", s.requireAdmin(s.setAllSchedules(false))).Methods("POST")
	api.HandleFunc("/schedules/{id}", s.updateSchedule).Methods("PUT")
	api.HandleFunc("/schedules/{id}", s.deleteSchedule).Methods("DELETE")
}

func (s *Server) registerAutomationRoutes(api *mux.Router) {
	api.HandleFunc("/automations", s.requireAdmin(s.getAutomations)).Methods("GET")
	api.HandleFunc("/automations", s.requireAdmin(s.createAutomation)).Methods("POST")
	api.HandleFunc("/automations/{id}", s.requireAdmin(s.updateAutomation)).Methods("PUT")
	api.HandleFunc("/automations/{id}", s.requireAdmin(s.deleteAutomation)).Methods("DELETE")
	api.HandleFunc("/automations/{id}/run", s.requireAdmin(s.runAutomation)).Methods("POST")
	api.HandleFunc("/automations/{id}/rules/{idx}/run", s.requireAdmin(s.runAutomationRule)).Methods("POST")
}

func (s *Server) registerGroupRoutes(api *mux.Router) {
	api.HandleFunc("/groups", s.requireAdmin(s.getGroups)).Methods("GET")
	api.HandleFunc("/groups", s.requireAdmin(s.createGroup)).Methods("POST")
	api.HandleFunc("/groups/{id}", s.requireAdmin(s.getGroup)).Methods("GET")
	api.HandleFunc("/groups/{id}", s.requireAdmin(s.updateGroup)).Methods("PUT")
	api.HandleFunc("/groups/{id}", s.requireAdmin(s.deleteGroup)).Methods("DELETE")
	api.HandleFunc("/groups/{id}/on", s.requireAdmin(s.groupAction("on"))).Methods("POST")
	api.HandleFunc("/groups/{id}/off", s.requireAdmin(s.groupAction("off"))).Methods("POST")
	api.HandleFunc("/groups/{id}/toggle", s.requireAdmin(s.groupAction("toggle"))).Methods("POST")
}

func (s *Server) registerSceneRoutes(api *mux.Router) {
	api.HandleFunc("/scenes", s.requireAdmin(s.getScenes)).Methods("GET")
	api.HandleFunc("/scenes", s.requireAdmin(s.createScene)).Methods("POST")
	api.HandleFunc("/scenes/{id}", s.requireAdmin(s.getScene)).Methods("GET")
	api.HandleFunc("/scenes/{id}", s.requireAdmin(s.updateScene)).Methods("PUT")
	api.HandleFunc("/scenes/{id}", s.requireAdmin(s.deleteScene)).Methods("DELETE")
	api.HandleFunc("/scenes/{id}/activate", s.requireAdmin(s.activateScene)).Methods("POST")
}

func (s *Server) registerTimerRoutes(api *mux.Router) {
	api.HandleFunc("/timers", s.requireAdmin(s.getTimers)).Methods("GET")
	api.HandleFunc("/timers", s.requireAdmin(s.createTimer)).Methods("POST")
	api.HandleFunc("/timers/{id}", s.requireAdmin(s.deleteTimer)).Methods("DELETE")
}

func (s *Server) registerSensorRoutes(api *mux.Router) {
	api.HandleFunc("/sensors", s.requireAdmin(s.getSensors)).Methods("GET")
	api.HandleFunc("/sensors", s.requireAdmin(s.createSensor)).Methods("POST")
	api.HandleFunc("/sensors/pair/start", s.requireAdmin(s.startSensorPair)).Methods("POST")
	api.HandleFunc("/sensors/discover", s.requireAdmin(s.listDiscoveryCandidates)).Methods("GET")
	api.HandleFunc("/sensors/{id}", s.requireAdmin(s.updateSensor)).Methods("PUT")
	api.HandleFunc("/sensors/{id}", s.requireAdmin(s.deleteSensor)).Methods("DELETE")
	api.HandleFunc("/sensors/{id}/readings", s.requireAdmin(s.getSensorReadings)).Methods("GET")
	api.HandleFunc("/sensors/{id}/readings", s.requireAdmin(s.postSensorReading)).Methods("POST")
}

// registerAdminRoutes mounts the whole-home management surface: activity log,
// settings and config import/export.
func (s *Server) registerAdminRoutes(api *mux.Router) {
	api.HandleFunc("/activity", s.requireAdmin(s.getActivity)).Methods("GET")
	api.HandleFunc("/shortcut-auth", s.requireAdmin(s.getShortcutAuth)).Methods("GET")

	api.HandleFunc("/settings", s.getSettings).Methods("GET")
	api.HandleFunc("/settings", s.requireAdmin(s.updateSettings)).Methods("PUT")

	api.HandleFunc("/export", s.requireAdmin(s.exportConfig)).Methods("GET")
	api.HandleFunc("/import", s.requireAdmin(s.importConfig)).Methods("POST")
}

func (s *Server) registerTasmotaRoutes(api *mux.Router) {
	api.HandleFunc("/tasmota/probe", s.requireAdmin(s.tasmotaProbe)).Methods("GET")
	api.HandleFunc("/tasmota/{socketId}", s.tasmotaGetState).Methods("GET")
	api.HandleFunc("/tasmota/{socketId}/state", s.tasmotaSetState).Methods("PUT")
}

// registerSonosRoutes mounts local UPnP control (playback, volume, grouping,
// favorites). Browse + playback control is open to admins and kid profiles —
// the kid surface is a music player too — while discovery, device management,
// settings and the event monitor stay admin-only.
func (s *Server) registerSonosRoutes(api *mux.Router) {
	api.HandleFunc("/sonos/status", s.requireAdminOrKid(s.sonosStatus)).Methods("GET")
	api.HandleFunc("/sonos/discover", s.requireAdmin(s.sonosDiscover)).Methods("GET")
	// Registered ahead of the /sonos/{id}/… routes so "events" is never
	// mistaken for a speaker id.
	api.HandleFunc("/sonos/events", s.requireAdmin(s.sonosEventHealth)).Methods("GET")
	api.HandleFunc("/sonos/events/retry", s.requireAdmin(s.sonosEventRetry)).Methods("POST")
	api.HandleFunc("/sonos/speakers", s.requireAdmin(s.sonosCreateSpeaker)).Methods("POST")
	api.HandleFunc("/sonos/speakers/{id}", s.requireAdmin(s.sonosUpdateSpeaker)).Methods("PUT")
	api.HandleFunc("/sonos/speakers/{id}", s.requireAdmin(s.sonosDeleteSpeaker)).Methods("DELETE")
	api.HandleFunc("/sonos/{id}/play", s.requireAdminOrKid(s.sonosTransport(sonos.Play))).Methods("POST")
	api.HandleFunc("/sonos/{id}/pause", s.requireAdminOrKid(s.sonosTransport(sonos.Pause))).Methods("POST")
	api.HandleFunc("/sonos/{id}/next", s.requireAdminOrKid(s.sonosTransport(sonos.Next))).Methods("POST")
	api.HandleFunc("/sonos/{id}/previous", s.requireAdminOrKid(s.sonosTransport(sonos.Previous))).Methods("POST")
	api.HandleFunc("/sonos/{id}/leave", s.requireAdminOrKid(s.sonosTransport(sonos.Leave))).Methods("POST")
	api.HandleFunc("/sonos/{id}/join", s.requireAdminOrKid(s.sonosJoin)).Methods("POST")
	api.HandleFunc("/sonos/{id}/volume", s.requireAdminOrKid(s.sonosSetVolume)).Methods("PUT")
	api.HandleFunc("/sonos/{id}/mute", s.requireAdminOrKid(s.sonosSetMute)).Methods("PUT")
	api.HandleFunc("/sonos/{id}/favorites", s.requireAdmin(s.sonosFavorites)).Methods("GET")
	api.HandleFunc("/sonos/{id}/favorites/play", s.requireAdmin(s.sonosPlayFavorite)).Methods("POST")
	api.HandleFunc("/sonos/{id}/art", s.requireAdminOrKid(s.sonosArt)).Methods("GET")
	api.HandleFunc("/sonos/{id}/image", s.requireAdmin(s.sonosImage)).Methods("GET")
	api.HandleFunc("/sonos/{id}/settings", s.requireAdmin(s.sonosSettings)).Methods("GET")
	api.HandleFunc("/sonos/{id}/settings", s.requireAdmin(s.sonosUpdateSettings)).Methods("PUT")
	api.HandleFunc("/sonos/{id}/play-item", s.requireAdminOrKid(s.sonosPlayItem)).Methods("POST")
	api.HandleFunc("/sonos/{id}/seek", s.requireAdminOrKid(s.sonosSeek)).Methods("PUT")
	api.HandleFunc("/sonos/{id}/playmode", s.requireAdminOrKid(s.sonosSetPlayMode)).Methods("PUT")
	api.HandleFunc("/sonos/{id}/crossfade", s.requireAdminOrKid(s.sonosSetCrossfade)).Methods("PUT")
	api.HandleFunc("/sonos/{id}/autoplay", s.requireAdminOrKid(s.sonosSetAutoplay)).Methods("PUT")
	api.HandleFunc("/sonos/{id}/queue", s.requireAdminOrKid(s.sonosQueue)).Methods("GET")
	api.HandleFunc("/sonos/{id}/queue", s.requireAdminOrKid(s.sonosQueueAdd)).Methods("POST")
	api.HandleFunc("/sonos/{id}/queue", s.requireAdminOrKid(s.sonosQueueClear)).Methods("DELETE")
	api.HandleFunc("/sonos/{id}/queue/{track}", s.requireAdminOrKid(s.sonosQueueRemove)).Methods("DELETE")
	api.HandleFunc("/sonos/{id}/queue/{track}", s.requireAdminOrKid(s.sonosQueueMove)).Methods("PUT")
}

// registerKEFRoutes mounts local HTTP control (transport, volume, source, DSP
// settings). No grouping, queue or favorites — the speaker's API has none of
// them; see internal/kef.
func (s *Server) registerKEFRoutes(api *mux.Router) {
	api.HandleFunc("/kef/status", s.requireAdmin(s.kefStatus)).Methods("GET")
	api.HandleFunc("/kef/discover", s.requireAdmin(s.kefDiscover)).Methods("GET")
	api.HandleFunc("/kef/speakers", s.requireAdmin(s.kefCreateSpeaker)).Methods("POST")
	api.HandleFunc("/kef/speakers/{id}", s.requireAdmin(s.kefUpdateSpeaker)).Methods("PUT")
	api.HandleFunc("/kef/speakers/{id}", s.requireAdmin(s.kefDeleteSpeaker)).Methods("DELETE")
	api.HandleFunc("/kef/{id}/play", s.requireAdmin(s.kefTransport(kef.Play))).Methods("POST")
	api.HandleFunc("/kef/{id}/pause", s.requireAdmin(s.kefTransport(kef.Pause))).Methods("POST")
	api.HandleFunc("/kef/{id}/next", s.requireAdmin(s.kefTransport(kef.Next))).Methods("POST")
	api.HandleFunc("/kef/{id}/previous", s.requireAdmin(s.kefTransport(kef.Previous))).Methods("POST")
	api.HandleFunc("/kef/{id}/volume", s.requireAdmin(s.kefSetVolume)).Methods("PUT")
	api.HandleFunc("/kef/{id}/mute", s.requireAdmin(s.kefSetMute)).Methods("PUT")
	api.HandleFunc("/kef/{id}/source", s.requireAdmin(s.kefSetSource)).Methods("PUT")
	api.HandleFunc("/kef/{id}/power", s.requireAdmin(s.kefSetPower)).Methods("PUT")
	api.HandleFunc("/kef/{id}/art", s.requireAdmin(s.kefArt)).Methods("GET")
	api.HandleFunc("/kef/{id}/settings", s.requireAdmin(s.kefSettings)).Methods("GET")
	api.HandleFunc("/kef/{id}/settings", s.requireAdmin(s.kefUpdateSettings)).Methods("PUT")
	// Starting music is the one KEF capability that isn't on the speaker's
	// own API: it goes through Spotify Connect. See kef_spotify.go.
	api.HandleFunc("/kef/{id}/play-item", s.requireAdmin(s.kefPlayItem)).Methods("POST")
	api.HandleFunc("/kef/{id}/spotify", s.requireAdmin(s.kefSpotifyDevices)).Methods("GET")
	api.HandleFunc("/kef/{id}/spotify", s.requireAdmin(s.kefSetSpotifyDevice)).Methods("PUT")
}

// registerMediaRoutes mounts the media protocol: speakers and services
// addressed uniformly, and zones — sets of speakers that play together
// regardless of make. The vendor routes above stay: they expose specifics
// (crossfade, KEF sources, Sonos queues) that don't generalise. See
// docs/MEDIA-PROTOCOL.md.
func (s *Server) registerMediaRoutes(api *mux.Router) {
	api.HandleFunc("/media/endpoints", s.requireAdmin(s.mediaEndpoints)).Methods("GET")
	api.HandleFunc("/media/providers", s.requireAdmin(s.mediaProviders)).Methods("GET")
	api.HandleFunc("/media/search", s.requireAdmin(s.mediaSearch)).Methods("GET")
	api.HandleFunc("/media/zones", s.requireAdmin(s.mediaZones)).Methods("GET")
	api.HandleFunc("/media/zones", s.requireAdmin(s.mediaCreateZone)).Methods("POST")
	api.HandleFunc("/media/zones/{id}", s.requireAdmin(s.mediaUpdateZone)).Methods("PUT")
	api.HandleFunc("/media/zones/{id}", s.requireAdmin(s.mediaDeleteZone)).Methods("DELETE")
	api.HandleFunc("/media/zones/{id}/routes", s.requireAdmin(s.mediaZoneRoutes)).Methods("GET")
	api.HandleFunc("/media/zones/{id}/play", s.requireAdmin(s.mediaZonePlay)).Methods("POST")
	api.HandleFunc("/media/history", s.requireAdminOrKid(s.mediaHistory)).Methods("GET")
	api.HandleFunc("/announce", s.requireAdmin(s.announceStatus)).Methods("GET")
	api.HandleFunc("/announce", s.requireAdmin(s.announceSend)).Methods("POST")
	api.HandleFunc("/media/zones/{id}/stop", s.requireAdmin(s.mediaZoneStop)).Methods("POST")
	api.HandleFunc("/media/zones/{id}/resume",
		s.requireAdmin(s.mediaZoneTransport(media.TransportPlay))).Methods("POST")
	api.HandleFunc("/media/zones/{id}/pause",
		s.requireAdmin(s.mediaZoneTransport(media.TransportPause))).Methods("POST")
	api.HandleFunc("/media/zones/{id}/next",
		s.requireAdmin(s.mediaZoneTransport(media.TransportNext))).Methods("POST")
	api.HandleFunc("/media/zones/{id}/previous",
		s.requireAdmin(s.mediaZoneTransport(media.TransportPrevious))).Methods("POST")
	api.HandleFunc("/media/zones/{id}/volume", s.requireAdmin(s.mediaZoneVolume)).Methods("PUT")
	api.HandleFunc("/media/zones/{id}/mute", s.requireAdmin(s.mediaZoneMute)).Methods("PUT")
}

// registerSpotifyRoutes mounts search/browse and account linking. OAuth is
// the caller's own account (PKCE): a kid profile links and searches as its
// own account, an admin as the household's. Sonos playback stays local via
// the play-item route above, while KEF's goes back out through Connect.
// The developer app's client ID stays admin-only, like every setup surface.
func (s *Server) registerSpotifyRoutes(api *mux.Router) {
	api.HandleFunc("/spotify/status", s.requireAdminOrKid(s.spotifyStatus)).Methods("GET")
	api.HandleFunc("/spotify/config", s.requireAdmin(s.spotifySetConfig)).Methods("PUT")
	api.HandleFunc("/spotify/login", s.requireAdminOrKid(s.spotifyLogin)).Methods("GET")
	api.HandleFunc("/spotify/callback", s.requireAdminOrKid(s.spotifyCallback)).Methods("GET")
	api.HandleFunc("/spotify/exchange", s.requireAdminOrKid(s.spotifyExchange)).Methods("POST")
	api.HandleFunc("/spotify/disconnect", s.requireAdminOrKid(s.spotifyDisconnect)).Methods("POST")
	api.HandleFunc("/spotify/search", s.requireAdminOrKid(s.spotifySearch)).Methods("GET")
	api.HandleFunc("/spotify/playlists", s.requireAdminOrKid(s.spotifyPlaylists)).Methods("GET")
	api.HandleFunc("/spotify/listening", s.requireAdminOrKid(s.spotifyListening)).Methods("GET")
	api.HandleFunc("/spotify/artist", s.requireAdminOrKid(s.spotifyArtist)).Methods("GET")
	api.HandleFunc("/spotify/context", s.requireAdminOrKid(s.spotifyContext)).Methods("GET")
	api.HandleFunc("/spotify/similar", s.requireAdminOrKid(s.spotifySimilar)).Methods("GET")
	api.HandleFunc("/spotify/saved", s.requireAdminOrKid(s.spotifySaved)).Methods("GET")
	api.HandleFunc("/spotify/saved", s.requireAdminOrKid(s.spotifySetSaved)).Methods("PUT")
}

func (s *Server) registerMatterRoutes(api *mux.Router) {
	api.HandleFunc("/matter/transport", s.requireAdmin(s.matterTransport)).Methods("GET")
	api.HandleFunc("/matter/devices", s.requireAdmin(s.matterListDevices)).Methods("GET")
	api.HandleFunc("/matter/commission", s.requireAdmin(s.matterCommission)).Methods("POST")
	api.HandleFunc("/matter/commission/jobs/{id}", s.requireAdmin(s.matterCommissionJob)).Methods("GET")
	api.HandleFunc("/matter/{socketId}", s.matterGetState).Methods("GET")
	api.HandleFunc("/matter/{socketId}/state", s.matterSetState).Methods("PUT")
}

func (s *Server) registerMQTTRoutes(api *mux.Router) {
	api.HandleFunc("/mqtt/status", s.requireAdmin(s.mqttStatus)).Methods("GET")
	api.HandleFunc("/mqtt/publish", s.requireAdmin(s.mqttPublish)).Methods("POST")
}

// registerAssistantRoutes mounts the local LLM assistant. Admin-gated: it can
// drive bulk control and reads across every device, matching the posture of
// the groups/scenes routes. When the LLM client is disabled the handlers
// return 503.
func (s *Server) registerAssistantRoutes(api *mux.Router) {
	api.HandleFunc("/assistant/status", s.requireAdmin(s.assistantStatus)).Methods("GET")
	api.HandleFunc("/assistant/chat", s.requireAdmin(s.assistantChat)).Methods("POST")
	api.HandleFunc("/assistant/confirm", s.requireAdmin(s.assistantConfirm)).Methods("POST")
}

// registerPublicRoutes mounts the routes that sit outside the authenticated
// /api subrouter, either because the caller has no session yet (login, invite,
// VAPID key) or because the caller is a device with no credentials at all
// (Sonos event callbacks, the audio stream).
func (s *Server) registerPublicRoutes(r *mux.Router) {
	// Auth endpoints are public — the SPA needs to reach /api/login without
	// being authenticated, and /api/logout just clears the cookie.
	r.HandleFunc("/api/login", s.handleLogin).Methods("POST")
	r.HandleFunc("/api/logout", s.handleLogout).Methods("POST")

	// Invite endpoints are also public: a new admin sets their own password
	// via a one-time link before they have a session cookie.
	r.HandleFunc("/api/invite", s.lookupInvite).Methods("GET")
	r.HandleFunc("/api/invite", s.acceptInvite).Methods("POST")
}

// registerPushRoutes mounts push notifications. vapid-key is public (no auth)
// so the frontend can subscribe before the user is authenticated.
// Subscribe/unsubscribe require a session; prefs require auth but not admin.
func (s *Server) registerPushRoutes(r, api *mux.Router) {
	r.HandleFunc("/api/push/vapid-key", s.getPushVAPIDKey).Methods("GET")
	api.HandleFunc("/push/subscribe", s.subscribePush).Methods("POST")
	api.HandleFunc("/push/unsubscribe", s.unsubscribePush).Methods("DELETE")
	api.HandleFunc("/push/prefs", s.updatePushPrefs).Methods("PUT")
	api.HandleFunc("/push/test", s.testPush).Methods("POST")
}

// registerDeviceCallbackRoutes mounts the two endpoints devices call back on.
// Both are unauthenticated by necessity — speakers have no credentials.
func (s *Server) registerDeviceCallbackRoutes(r *mux.Router) {
	// Sonos change notifications. Guarded by the unguessable token, the SID
	// and a source-address check; see handleSonosEvent. Bound to NOTIFY only,
	// so a browser hitting the same path still falls through to the SPA.
	r.HandleFunc(sonosEventPath+"/{token}", s.handleSonosEvent).Methods("NOTIFY")

	// The audio stream speakers pull from on the cross-vendor route. Guarded
	// by the stream id — 128 bits of randomness, minted per playback and
	// invalid the moment it ends. GET and HEAD only, since speakers probe with
	// HEAD before committing to play.
	r.PathPrefix(streamPath+"/").Handler(s.streamHandler()).Methods("GET", "HEAD")

	// The announcement clip a speaker fetches when the house is being
	// called. Same posture as the stream: unguarded by the session because
	// the client is a speaker, guarded instead by an unguessable id that
	// stops mattering a couple of minutes after it is minted.
	r.PathPrefix(announcePath+"/").Handler(s.announceHandler()).Methods("GET", "HEAD")
}
