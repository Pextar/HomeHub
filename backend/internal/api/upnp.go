package api

// UPnP/DLNA renderer registration.
//
// Deliberately thinner than the other speaker surfaces, because a renderer is a
// thinner thing. There is no vendor API to expose, no queue, no EQ, no source
// selection — a renderer is a box that fetches a URL and plays it. What it adds
// to this house is the one capability no other endpoint has: it reads the WAV
// header rather than being bound to a push protocol's fixed rate, so it is the
// only way HomeHub can deliver 24-bit/192 kHz to hardware that can play it.
//
// The registration flow is a describe rather than a probe. A renderer publishes
// its control URLs inside a device description at a URL of its choosing, so
// adding one means fetching that document and remembering what it said. Sonos
// and KEF can be found by address alone because their ports and paths are
// fixed; this cannot.

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"homehub/internal/store"
	"homehub/internal/upnp"
)

// upnpRenderers handles GET /api/upnp/renderers.
func (s *Server) upnpRenderers(w http.ResponseWriter, r *http.Request) {
	out := store.ViewValue(s.Store, func() []store.UPnPRenderer {
		list := make([]store.UPnPRenderer, 0, len(s.Store.UPnP))
		for _, rn := range s.Store.UPnP {
			list = append(list, *rn)
		}
		return list
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, http.StatusOK, out)
}

// upnpDescribe handles POST /api/upnp/describe — read a device description
// without registering anything.
//
// Separate from creation so the UI can show what was found and let someone
// confirm it is the right box before it joins the house. It also reports what
// the renderer says it can play, which is the one thing worth knowing before
// committing: a renderer that does not list PCM or WAV probably cannot be
// served losslessly, and finding that out at setup beats finding it as silence.
func (s *Server) upnpDescribe(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Location string `json:"location"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	dev, err := upnp.Describe(ctx, strings.TrimSpace(body.Location))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	// Advisory, and a failure here is not fatal: plenty of renderers answer
	// the transport calls perfectly and have a broken ConnectionManager.
	proto, _ := upnp.Protocols(ctx, dev)

	writeJSON(w, http.StatusOK, map[string]any{
		"name":         dev.Name,
		"manufacturer": dev.Manufacturer,
		"model":        dev.Model,
		"udn":          dev.UDN,
		"plays_pcm":    proto.PlaysPCM(),
		"formats":      proto.Raw,
	})
}

// upnpCreateRenderer handles POST /api/upnp/renderers.
func (s *Server) upnpCreateRenderer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Location string `json:"location"`
		Name     string `json:"name"`
		Room     string `json:"room"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	location := strings.TrimSpace(body.Location)
	if location == "" {
		writeError(w, http.StatusBadRequest, "the renderer's description URL is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	dev, err := upnp.Describe(ctx, location)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	proto, _ := upnp.Protocols(ctx, dev)

	host, port := upnp.HostPort(dev.AVTransportURL)
	rn := store.UPnPRenderer{
		Name:                strings.TrimSpace(body.Name),
		Room:                strings.TrimSpace(body.Room),
		IP:                  host,
		Port:                port,
		Location:            location,
		UDN:                 dev.UDN,
		Model:               strings.TrimSpace(dev.Manufacturer + " " + dev.Model),
		AVTransportURL:      dev.AVTransportURL,
		RenderingControlURL: dev.RenderingControlURL,
		ConnectionMgrURL:    dev.ConnectionMgrURL,
		PlaysPCM:            proto.PlaysPCM(),
	}
	if rn.Name == "" {
		rn.Name = dev.Name
	}

	if !s.update(w, func() error {
		rn.ID = fmt.Sprintf("upnp_%d", time.Now().UnixNano())
		if err := s.Store.ValidateUPnPRenderer(&rn); err != nil {
			return errInvalid(err)
		}
		s.Store.UPnP[rn.ID] = &rn
		return nil
	}) {
		return
	}
	writeJSON(w, http.StatusCreated, rn)
}

// upnpUpdateRenderer handles PUT /api/upnp/renderers/{id}. Name and room are
// the household's to choose; the control URLs are the device's own answer and
// are refreshed by re-describing rather than edited.
func (s *Server) upnpUpdateRenderer(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var updates store.UPnPRenderer
	if !decodeBody(w, r, &updates) {
		return
	}
	var existing *store.UPnPRenderer
	if !s.update(w, func() error {
		var ok bool
		existing, ok = s.Store.UPnP[id]
		if !ok {
			return errStatus(http.StatusNotFound, "renderer not found")
		}
		merged := *existing
		if v := strings.TrimSpace(updates.Name); v != "" {
			merged.Name = v
		}
		merged.Room = strings.TrimSpace(updates.Room)
		if err := s.Store.ValidateUPnPRenderer(&merged); err != nil {
			return errInvalid(err)
		}
		*existing = merged
		return nil
	}) {
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

// upnpRefreshRenderer handles POST /api/upnp/renderers/{id}/refresh — re-read
// the device description and update the stored control URLs.
//
// Worth having as its own action because the failure it fixes is silent: a
// renderer that reboots onto a different port keeps answering discovery and
// stops answering the URLs HomeHub remembered, and every play then fails with a
// connection error that says nothing about why.
func (s *Server) upnpRefreshRenderer(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	location := store.ViewValue(s.Store, func() string {
		if rn, ok := s.Store.UPnP[id]; ok {
			return rn.Location
		}
		return ""
	})
	if location == "" {
		// Covers both "no such renderer" and "stored without a location".
		// The second cannot happen through the API — creation requires one —
		// so a hand-edited file is the only way here, and the message says
		// what to do rather than which of the two it was.
		writeError(w, http.StatusNotFound,
			"no renderer with a description URL to refresh — remove it and add it again")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	dev, err := upnp.Describe(ctx, location)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	proto, _ := upnp.Protocols(ctx, dev)

	var updated *store.UPnPRenderer
	if !s.update(w, func() error {
		rn, ok := s.Store.UPnP[id]
		if !ok {
			return errStatus(http.StatusNotFound, "renderer not found")
		}
		merged := *rn
		merged.AVTransportURL = dev.AVTransportURL
		merged.RenderingControlURL = dev.RenderingControlURL
		merged.ConnectionMgrURL = dev.ConnectionMgrURL
		merged.UDN = dev.UDN
		merged.PlaysPCM = proto.PlaysPCM()
		merged.IP, merged.Port = upnp.HostPort(dev.AVTransportURL)
		if err := s.Store.ValidateUPnPRenderer(&merged); err != nil {
			return errInvalid(err)
		}
		*rn = merged
		updated = rn
		return nil
	}) {
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// upnpDeleteRenderer handles DELETE /api/upnp/renderers/{id}.
func (s *Server) upnpDeleteRenderer(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if !s.update(w, func() error {
		if _, ok := s.Store.UPnP[id]; !ok {
			return errNotFound("renderer")
		}
		delete(s.Store.UPnP, id)
		// Drop it from any zone that held it, same as every other speaker.
		s.Store.CascadeDeleteSpeaker(store.QualifyUPnP(id))
		return nil
	}) {
		return
	}
	s.pruneDeadRooms()
	w.WriteHeader(http.StatusNoContent)
}
