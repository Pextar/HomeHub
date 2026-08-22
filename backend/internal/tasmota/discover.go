package tasmota

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"
)

// Discovery sweeps the LAN rather than asking mDNS, and that is deliberate.
// Tasmota's mDNS responder is behind the USE_DISCOVERY compile flag, which
// the stock tasmota.bin ships with OFF to save flash — so a DNS-SD query
// finds nothing on most real devices. Every Tasmota build does answer
// /cm?cmnd=Status on port 80, so probing the host's own subnets is both
// simpler and finds strictly more devices. Same shape as sonos.Discover /
// kef.Discover: a bounded scan that returns what answered in the window.
const (
	// discoverProbeTimeout caps one host probe. Deliberately short — a live
	// device on the LAN answers in single-digit milliseconds, and this
	// number multiplies out across the whole sweep for every address that
	// has nothing on it.
	discoverProbeTimeout = 900 * time.Millisecond
	// discoverWorkers is how many probes are in flight at once.
	discoverWorkers = 64
	// maxSweepHosts caps the total addresses one Discover call will probe,
	// so a host sitting on a wide subnet can't turn this into a port scan
	// of tens of thousands of addresses.
	maxSweepHosts = 2048
	// maxStatusBytes caps one probe's response body. Something else on port
	// 80 could stream forever; a Tasmota status is a couple of kilobytes.
	maxStatusBytes = 64 << 10
)

// Device is one Tasmota device found on the LAN.
type Device struct {
	IP    string `json:"ip"`
	Name  string `json:"name,omitempty"`
	Topic string `json:"topic,omitempty"`
}

// statusResponse is the part of cmnd=Status we read. Tasmota answers with
// every field a build has; we want the human-facing name and the MQTT topic.
type statusResponse struct {
	Status struct {
		DeviceName   string   `json:"DeviceName"`
		FriendlyName []string `json:"FriendlyName"`
		Topic        string   `json:"Topic"`
	} `json:"Status"`
}

// Discover probes every address on the host's own IPv4 subnets and returns
// the ones that answer as Tasmota, sorted by IP. ctx bounds the whole sweep;
// callers should give it a few seconds.
//
// An empty result is not an error — it means nothing answered.
func Discover(ctx context.Context) ([]Device, error) {
	targets, err := sweepTargets()
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return []Device{}, nil
	}

	var (
		mu    sync.Mutex
		found []Device
		wg    sync.WaitGroup
	)
	work := make(chan string)

	workers := discoverWorkers
	if len(targets) < workers {
		workers = len(targets)
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ip := range work {
				dev, ok := probeDevice(ctx, ip)
				if !ok {
					continue
				}
				mu.Lock()
				found = append(found, dev)
				mu.Unlock()
			}
		}()
	}

	for _, ip := range targets {
		select {
		case <-ctx.Done():
			close(work)
			wg.Wait()
			return sortDevices(found), nil
		case work <- ip:
		}
	}
	close(work)
	wg.Wait()
	return sortDevices(found), nil
}

// probeDevice asks one address for its status and reports whether a Tasmota
// answered. Anything that isn't valid Tasmota JSON is not a Tasmota device —
// plenty of things on a home LAN answer port 80.
func probeDevice(ctx context.Context, ip string) (Device, bool) {
	cctx, cancel := context.WithTimeout(ctx, discoverProbeTimeout)
	defer cancel()

	u := fmt.Sprintf("http://%s/cm?cmnd=Status", ip)
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, u, nil)
	if err != nil {
		return Device{}, false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Device{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Device{}, false
	}
	// Cap the read: something else on port 80 could stream indefinitely.
	var status statusResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxStatusBytes)).Decode(&status); err != nil {
		return Device{}, false
	}
	// A device that parsed but carries no topic isn't Tasmota — every
	// Tasmota build sets one, and this is what rejects unrelated JSON APIs.
	if status.Status.Topic == "" {
		return Device{}, false
	}
	name := status.Status.DeviceName
	if name == "" && len(status.Status.FriendlyName) > 0 {
		name = status.Status.FriendlyName[0]
	}
	return Device{IP: ip, Name: name, Topic: status.Status.Topic}, true
}

// sweepTargets lists every unicast IPv4 address on the host's own subnets,
// skipping the network and broadcast addresses and our own interfaces.
func sweepTargets() ([]string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("tasmota: list interfaces: %w", err)
	}
	seen := make(map[string]bool)
	var out []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipnet.IP.To4()
			if ip4 == nil || !ip4.IsPrivate() {
				continue
			}
			ones, bits := ipnet.Mask.Size()
			if bits != 32 {
				continue
			}
			// Refuse anything wider than a /22 — beyond that this stops
			// being "my home network" and becomes a scan.
			if ones < 22 {
				continue
			}
			for _, host := range hostsIn(ipnet) {
				if host == ip4.String() || seen[host] {
					continue
				}
				seen[host] = true
				out = append(out, host)
				if len(out) >= maxSweepHosts {
					return out, nil
				}
			}
		}
	}
	return out, nil
}

// hostsIn enumerates the usable host addresses in a subnet, excluding the
// network and broadcast addresses.
func hostsIn(ipnet *net.IPNet) []string {
	ip := ipnet.IP.To4()
	mask := net.IP(ipnet.Mask).To4()
	if ip == nil || mask == nil {
		return nil
	}
	network := make(net.IP, 4)
	broadcast := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		network[i] = ip[i] & mask[i]
		broadcast[i] = network[i] | ^mask[i]
	}
	start := ipToUint(network) + 1
	end := ipToUint(broadcast)
	if end <= start {
		return nil
	}
	out := make([]string, 0, end-start)
	for v := start; v < end; v++ {
		out = append(out, uintToIP(v).String())
	}
	return out
}

func ipToUint(ip net.IP) uint32 {
	ip = ip.To4()
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

func uintToIP(v uint32) net.IP {
	return net.IPv4(byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

func sortDevices(devs []Device) []Device {
	if devs == nil {
		return []Device{}
	}
	sort.Slice(devs, func(i, j int) bool {
		return ipToUint(net.ParseIP(devs[i].IP)) < ipToUint(net.ParseIP(devs[j].IP))
	})
	return devs
}
