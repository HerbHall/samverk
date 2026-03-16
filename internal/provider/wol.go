package provider

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// WoLConfig describes Wake-on-LAN settings for a provider host.
type WoLConfig struct {
	MAC string `yaml:"wol_mac"` // e.g., "AA:BB:CC:DD:EE:FF"
}

// wolCooldownDuration is the minimum interval between WoL packets for the
// same target to avoid flooding the network.
const wolCooldownDuration = 5 * time.Minute

// WakeOnLAN sends a magic packet to wake a sleeping host.
// macAddr should be in "AA:BB:CC:DD:EE:FF" format.
func WakeOnLAN(macAddr string) error {
	mac, err := net.ParseMAC(macAddr)
	if err != nil {
		return fmt.Errorf("parse MAC address %q: %w", macAddr, err)
	}

	if len(mac) != 6 {
		return fmt.Errorf("MAC address %q has %d bytes, expected 6", macAddr, len(mac))
	}

	packet := buildMagicPacket(mac)

	conn, err := net.DialUDP("udp4", nil, &net.UDPAddr{
		IP:   net.IPv4bcast,
		Port: 9, // standard WoL port
	})
	if err != nil {
		return fmt.Errorf("dial UDP broadcast: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err = conn.Write(packet); err != nil {
		return fmt.Errorf("send magic packet: %w", err)
	}
	return nil
}

// buildMagicPacket constructs a 102-byte Wake-on-LAN magic packet:
// 6 bytes of 0xFF followed by the MAC address repeated 16 times.
func buildMagicPacket(mac net.HardwareAddr) []byte {
	packet := make([]byte, 102)

	// 6 bytes of 0xFF header.
	for i := 0; i < 6; i++ {
		packet[i] = 0xFF
	}

	// MAC address repeated 16 times.
	for i := 0; i < 16; i++ {
		copy(packet[6+i*6:], mac)
	}

	return packet
}

// wolCooldownTracker prevents sending WoL packets too frequently to the
// same MAC address.
type wolCooldownTracker struct {
	mu       sync.Mutex
	lastSent map[string]time.Time
}

func newWoLCooldownTracker() *wolCooldownTracker {
	return &wolCooldownTracker{
		lastSent: make(map[string]time.Time),
	}
}

// shouldSend returns true if enough time has passed since the last WoL
// packet was sent to the given MAC address. If true, it also records
// the current time as the last send time.
func (t *wolCooldownTracker) shouldSend(mac string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if last, ok := t.lastSent[mac]; ok {
		if time.Since(last) < wolCooldownDuration {
			return false
		}
	}
	t.lastSent[mac] = time.Now()
	return true
}
