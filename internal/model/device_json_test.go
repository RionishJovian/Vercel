package model

import (
	"encoding/json"
	"net"
	"strings"
	"testing"
)

func TestDeviceMACIsTextInJSON(t *testing.T) {
	mac, _ := net.ParseMAC("02:00:00:00:00:40")
	ip := net.ParseIP("192.168.88.20")
	raw, err := json.Marshal(&Device{DeviceID: "d1", HardwareName: "AP", MACAddress: &mac, ManagementIP: &ip})
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{`"mac_address":"02:00:00:00:00:40"`, `"management_ip":"192.168.88.20"`, `"device_id":"d1"`, `"hardware_name":"AP"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s in %s", want, text)
		}
	}
	if raw, err = json.Marshal(Device{DeviceID: "d2"}); err != nil || strings.Contains(string(raw), "mac_address") {
		t.Fatalf("no MAC must stay absent: %s %v", raw, err)
	}
}
