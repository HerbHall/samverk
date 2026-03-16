package provider

import (
	"net"
	"testing"
	"time"
)

func TestBuildMagicPacket_Length(t *testing.T) {
	mac, err := net.ParseMAC("AA:BB:CC:DD:EE:FF")
	if err != nil {
		t.Fatal(err)
	}

	packet := buildMagicPacket(mac)
	if len(packet) != 102 {
		t.Fatalf("expected 102 bytes, got %d", len(packet))
	}
}

func TestBuildMagicPacket_Header(t *testing.T) {
	mac, err := net.ParseMAC("11:22:33:44:55:66")
	if err != nil {
		t.Fatal(err)
	}

	packet := buildMagicPacket(mac)

	// First 6 bytes must be 0xFF.
	for i := 0; i < 6; i++ {
		if packet[i] != 0xFF {
			t.Fatalf("byte %d: expected 0xFF, got 0x%02X", i, packet[i])
		}
	}
}

func TestBuildMagicPacket_MACRepeated(t *testing.T) {
	mac, err := net.ParseMAC("11:22:33:44:55:66")
	if err != nil {
		t.Fatal(err)
	}

	packet := buildMagicPacket(mac)

	// MAC address should be repeated 16 times starting at offset 6.
	for rep := 0; rep < 16; rep++ {
		offset := 6 + rep*6
		for b := 0; b < 6; b++ {
			if packet[offset+b] != mac[b] {
				t.Fatalf("repetition %d, byte %d: expected 0x%02X, got 0x%02X",
					rep, b, mac[b], packet[offset+b])
			}
		}
	}
}

func TestWakeOnLAN_InvalidMAC(t *testing.T) {
	tests := []struct {
		name string
		mac  string
	}{
		{name: "empty", mac: ""},
		{name: "garbage", mac: "not-a-mac"},
		{name: "too_short", mac: "AA:BB:CC"},
		{name: "invalid_hex", mac: "GG:HH:II:JJ:KK:LL"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := WakeOnLAN(tc.mac)
			if err == nil {
				t.Fatal("expected error for invalid MAC")
			}
		})
	}
}

func TestWakeOnLAN_ValidMACFormats(t *testing.T) {
	// These should parse successfully (though sending may fail on CI
	// without a real NIC -- we only test that parsing works).
	tests := []struct {
		name string
		mac  string
	}{
		{name: "colon_separated", mac: "AA:BB:CC:DD:EE:FF"},
		{name: "dash_separated", mac: "AA-BB-CC-DD-EE-FF"},
		{name: "lowercase", mac: "aa:bb:cc:dd:ee:ff"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mac, err := net.ParseMAC(tc.mac)
			if err != nil {
				t.Fatalf("expected valid MAC parse, got: %v", err)
			}
			if len(mac) != 6 {
				t.Fatalf("expected 6 bytes, got %d", len(mac))
			}
		})
	}
}

func TestWoLCooldownTracker(t *testing.T) {
	tracker := newWoLCooldownTracker()

	// First send should be allowed.
	if !tracker.shouldSend("AA:BB:CC:DD:EE:FF") {
		t.Fatal("first send should be allowed")
	}

	// Immediate second send should be blocked by cooldown.
	if tracker.shouldSend("AA:BB:CC:DD:EE:FF") {
		t.Fatal("second immediate send should be blocked")
	}

	// Different MAC should be independent.
	if !tracker.shouldSend("11:22:33:44:55:66") {
		t.Fatal("different MAC should be allowed independently")
	}
}

func TestWoLCooldownTracker_ExpiredCooldown(t *testing.T) {
	tracker := newWoLCooldownTracker()

	// Set a past timestamp to simulate expired cooldown.
	tracker.mu.Lock()
	tracker.lastSent["AA:BB:CC:DD:EE:FF"] = time.Now().Add(-10 * time.Minute)
	tracker.mu.Unlock()

	if !tracker.shouldSend("AA:BB:CC:DD:EE:FF") {
		t.Fatal("send should be allowed after cooldown expires")
	}
}
