package api

import (
	"sort"
	"strings"

	"rf-socket-controller/internal/store"
)

// assistant_context.go builds the compact, read-only snapshot of the home
// that is injected into the model's system prompt, and resolves the
// human-friendly names the model emits ("kitchen lamp") back to entity ids.
// Everything here reads under a single RLock and never mutates the store.

// deviceLite is the trimmed device shape the model sees. Ids are included so
// the model can pass them straight back to a tool, avoiding a resolution round
// trip; names/rooms let it match what the user said.
type deviceLite struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Room     string `json:"room,omitempty"`
	State    string `json:"state"` // "on" | "off"
	Protocol string `json:"protocol"`
}

type roomLite struct {
	Name    string `json:"name"`
	Devices int    `json:"devices"`
	On      int    `json:"on"`
}

type sceneLite struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Room string `json:"room,omitempty"`
}

type groupLite struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Devices int    `json:"devices"`
}

type sensorLite struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Kind  string   `json:"kind,omitempty"`
	Unit  string   `json:"unit,omitempty"`
	Value *float64 `json:"value,omitempty"`
}

// stateSnapshot is the whole-home view embedded in the system prompt. Kept
// deliberately small (ids + names + current state) so it fits a 3B model's
// modest context window.
type stateSnapshot struct {
	Devices []deviceLite `json:"devices"`
	Rooms   []roomLite   `json:"rooms"`
	Scenes  []sceneLite  `json:"scenes"`
	Groups  []groupLite  `json:"groups"`
	Sensors []sensorLite `json:"sensors"`
}

// buildSnapshot assembles the snapshot under a single read lock. Devices are
// filtered by the user's access; scenes/groups/sensors are admin-only routes
// already, so the assistant (admin-gated) sees them all. Caller must NOT hold Mu.
func (s *Server) buildSnapshot(user *store.User) stateSnapshot {
	s.Store.Mu.RLock()
	defer s.Store.Mu.RUnlock()

	snap := stateSnapshot{
		Devices: make([]deviceLite, 0, len(s.Store.Sockets)),
		Scenes:  make([]sceneLite, 0, len(s.Store.Scenes)),
		Groups:  make([]groupLite, 0, len(s.Store.Groups)),
		Sensors: make([]sensorLite, 0, len(s.Store.Sensors)),
	}

	type counts struct{ total, on int }
	byRoom := make(map[string]*counts)
	for _, sock := range s.Store.Sockets {
		if !canAccess(user, sock.ID) {
			continue
		}
		snap.Devices = append(snap.Devices, deviceLite{
			ID:       sock.ID,
			Name:     sock.Name,
			Room:     sock.Room,
			State:    onOff(sock.State),
			Protocol: sock.Protocol,
		})
		if key := strings.ToLower(strings.TrimSpace(sock.Room)); key != "" {
			if byRoom[key] == nil {
				byRoom[key] = &counts{}
			}
			byRoom[key].total++
			if sock.State {
				byRoom[key].on++
			}
		}
	}
	for _, rm := range s.Store.Rooms {
		c := byRoom[strings.ToLower(rm.Name)]
		rl := roomLite{Name: rm.Name}
		if c != nil {
			rl.Devices = c.total
			rl.On = c.on
		}
		snap.Rooms = append(snap.Rooms, rl)
	}
	for _, sc := range s.Store.Scenes {
		snap.Scenes = append(snap.Scenes, sceneLite{ID: sc.ID, Name: sc.Name, Room: sc.Room})
	}
	for _, g := range s.Store.Groups {
		snap.Groups = append(snap.Groups, groupLite{ID: g.ID, Name: g.Name, Devices: len(g.SocketIDs)})
	}
	for _, sn := range s.Store.Sensors {
		snap.Sensors = append(snap.Sensors, sensorLite{
			ID: sn.ID, Name: sn.Name, Kind: sn.Kind, Unit: sn.Unit, Value: sn.LastValue,
		})
	}

	sort.Slice(snap.Devices, func(i, j int) bool { return less(snap.Devices[i].Name, snap.Devices[j].Name) })
	sort.Slice(snap.Rooms, func(i, j int) bool { return less(snap.Rooms[i].Name, snap.Rooms[j].Name) })
	sort.Slice(snap.Scenes, func(i, j int) bool { return less(snap.Scenes[i].Name, snap.Scenes[j].Name) })
	sort.Slice(snap.Groups, func(i, j int) bool { return less(snap.Groups[i].Name, snap.Groups[j].Name) })
	sort.Slice(snap.Sensors, func(i, j int) bool { return less(snap.Sensors[i].Name, snap.Sensors[j].Name) })
	return snap
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

func less(a, b string) bool { return strings.ToLower(a) < strings.ToLower(b) }

// resolveSocket maps a reference (id or name, optionally "room/name") to a
// socket the user may access. It returns the matched socket, or a non-empty
// reason string the tool surfaces back to the model (so it can disambiguate or
// pick differently) — never a Go error, so the agent loop keeps running.
// Caller must NOT hold Mu.
func (s *Server) resolveSocket(user *store.User, ref string) (sock store.Socket, ok bool, reason string) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return store.Socket{}, false, "no device specified"
	}
	s.Store.Mu.RLock()
	defer s.Store.Mu.RUnlock()

	// Exact id first.
	if d, found := s.Store.Sockets[ref]; found && canAccess(user, d.ID) {
		return *d, true, ""
	}

	// Case-insensitive name, then "room/name" form. Collect matches so an
	// ambiguous name can tell the model exactly which ones collided.
	lower := strings.ToLower(ref)
	var matches []*store.Socket
	for _, d := range s.Store.Sockets {
		if !canAccess(user, d.ID) {
			continue
		}
		name := strings.ToLower(d.Name)
		combined := strings.ToLower(strings.TrimSpace(d.Room) + "/" + d.Name)
		if name == lower || combined == lower {
			matches = append(matches, d)
		}
	}
	switch len(matches) {
	case 1:
		return *matches[0], true, ""
	case 0:
		return store.Socket{}, false, "no device named " + quote(ref)
	default:
		return store.Socket{}, false, ambiguityReason("device", ref, deviceLabels(matches))
	}
}

// resolveRoom maps a reference to a canonical room name. Caller must NOT hold Mu.
func (s *Server) resolveRoom(ref string) (name string, ok bool, reason string) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", false, "no room specified"
	}
	s.Store.Mu.RLock()
	defer s.Store.Mu.RUnlock()
	lower := strings.ToLower(ref)
	for _, rm := range s.Store.Rooms {
		if rm.ID == ref || strings.ToLower(rm.Name) == lower {
			return rm.Name, true, ""
		}
	}
	return "", false, "no room named " + quote(ref)
}

// resolveGroup maps a reference to a group id. Caller must NOT hold Mu.
func (s *Server) resolveGroup(ref string) (id, name string, ok bool, reason string) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "", false, "no group specified"
	}
	s.Store.Mu.RLock()
	defer s.Store.Mu.RUnlock()
	if g, found := s.Store.Groups[ref]; found {
		return g.ID, g.Name, true, ""
	}
	lower := strings.ToLower(ref)
	var matches []*store.Group
	for _, g := range s.Store.Groups {
		if strings.ToLower(g.Name) == lower {
			matches = append(matches, g)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0].ID, matches[0].Name, true, ""
	case 0:
		return "", "", false, "no group named " + quote(ref)
	default:
		labels := make([]string, len(matches))
		for i, g := range matches {
			labels[i] = g.Name
		}
		return "", "", false, ambiguityReason("group", ref, labels)
	}
}

// resolveScene maps a reference to a scene id. Caller must NOT hold Mu.
func (s *Server) resolveScene(ref string) (id, name string, ok bool, reason string) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "", false, "no scene specified"
	}
	s.Store.Mu.RLock()
	defer s.Store.Mu.RUnlock()
	if sc, found := s.Store.Scenes[ref]; found {
		return sc.ID, sc.Name, true, ""
	}
	lower := strings.ToLower(ref)
	var matches []*store.Scene
	for _, sc := range s.Store.Scenes {
		if strings.ToLower(sc.Name) == lower {
			matches = append(matches, sc)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0].ID, matches[0].Name, true, ""
	case 0:
		return "", "", false, "no scene named " + quote(ref)
	default:
		labels := make([]string, len(matches))
		for i, sc := range matches {
			labels[i] = sc.Name
		}
		return "", "", false, ambiguityReason("scene", ref, labels)
	}
}

// resolveSensor maps a reference to a sensor id. Caller must NOT hold Mu.
func (s *Server) resolveSensor(ref string) (id, name string, ok bool, reason string) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "", false, "no sensor specified"
	}
	s.Store.Mu.RLock()
	defer s.Store.Mu.RUnlock()
	if sn, found := s.Store.Sensors[ref]; found {
		return sn.ID, sn.Name, true, ""
	}
	lower := strings.ToLower(ref)
	var matches []*store.Sensor
	for _, sn := range s.Store.Sensors {
		if strings.ToLower(sn.Name) == lower {
			matches = append(matches, sn)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0].ID, matches[0].Name, true, ""
	case 0:
		return "", "", false, "no sensor named " + quote(ref)
	default:
		labels := make([]string, len(matches))
		for i, sn := range matches {
			labels[i] = sn.Name
		}
		return "", "", false, ambiguityReason("sensor", ref, labels)
	}
}

func deviceLabels(matches []*store.Socket) []string {
	labels := make([]string, len(matches))
	for i, d := range matches {
		if d.Room != "" {
			labels[i] = d.Room + "/" + d.Name + " (" + d.ID + ")"
		} else {
			labels[i] = d.Name + " (" + d.ID + ")"
		}
	}
	return labels
}

func ambiguityReason(kind, ref string, labels []string) string {
	return "more than one " + kind + " matches " + quote(ref) +
		": " + strings.Join(labels, ", ") + " — ask the user which one, or pass the id"
}

func quote(s string) string { return "\"" + s + "\"" }
