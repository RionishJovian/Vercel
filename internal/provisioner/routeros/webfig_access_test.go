package routeros

import (
	"context"
	"strings"
	"testing"
)

const webFigTestTunnel = "mmh-0123456789ab"

type webFigScriptRunner struct {
	services string
	rules    int
	commands []string
}

func (r *webFigScriptRunner) Run(_ context.Context, command string) (string, error) {
	r.commands = append(r.commands, command)
	switch {
	case strings.HasPrefix(command, "/interface/wireguard print count-only"):
		return "1", nil
	case strings.HasPrefix(command, ":put [/system/resource/get version]"):
		return "7.25 (stable)\n", nil
	case command == webFigReadServices:
		return r.services, nil
	case strings.HasPrefix(command, "/ip/firewall/filter print count-only"):
		if r.rules > 0 {
			return "1", nil
		}
		return "0", nil
	case strings.Contains(command, "/ip/firewall/filter add"):
		r.rules++
	case strings.Contains(command, "/ip/firewall/filter remove"):
		r.rules = 0
	case strings.Contains(command, "available-from="):
		value := command[strings.Index(command, "available-from=")+15:]
		value = value[:strings.Index(value, "]")]
		r.services = "mm|www|80|false|" + strings.ReplaceAll(value, ",", ";") + "\n"
	case strings.Contains(command, "disabled=no"):
		r.services = strings.Replace(r.services, "|true|", "|false|", 1)
	}
	return "", nil
}

func TestWebFigReadChangesNothingAndPlansOneRule(t *testing.T) {
	r := &webFigScriptRunner{services: "mm|www|80|false|\nmm|www-ssl|443|true|\n"}
	access, err := readWebFigAccess(context.Background(), r, webFigTestTunnel, "10.77.0.1/32")
	if err != nil {
		t.Fatal(err)
	}
	if access.Ready || access.Scheme != "http" || access.Port != 80 || access.Version != "7.25" || len(access.Plan) != 1 {
		t.Fatalf("%+v", access)
	}
	for _, command := range r.commands {
		if strings.Contains(command, " add ") || strings.Contains(command, " set ") || strings.Contains(command, " remove ") {
			t.Fatalf("read wrote: %s", command)
		}
	}
}

func TestWebFigUsesTheTunnelWhenOnlyHTTPSIsOn(t *testing.T) {
	r := &webFigScriptRunner{services: "mm|www|80|true|\nmm|www-ssl|8443|false|10.77.0.0/24\n", rules: 1}
	access, err := readWebFigAccess(context.Background(), r, webFigTestTunnel, "10.77.0.1/32")
	if err != nil {
		t.Fatal(err)
	}
	if access.Ready || !access.enableWWW || access.Scheme != "http" || access.Port != 80 || len(access.Plan) == 0 {
		t.Fatalf("an HTTPS-only router must get a reviewed plan, not a dead end: %+v", access)
	}
}

func TestWebFigPlanSaysWhenTheServiceMustChange(t *testing.T) {
	r := &webFigScriptRunner{services: "mm|www|80|false|192.168.88.0/24\n"}
	access, err := readWebFigAccess(context.Background(), r, webFigTestTunnel, "10.77.0.1/32")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(access.Plan, " "), "allowed to use the www service") || len(access.Plan) != 2 {
		t.Fatalf("%+v", access.Plan)
	}
	r = &webFigScriptRunner{services: "mm|www|80|true|\nmm|www-ssl|443|true|\n"}
	access, err = readWebFigAccess(context.Background(), r, webFigTestTunnel, "10.77.0.1/32")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(access.Plan, " "), "Turn on the router's web service") {
		t.Fatalf("%+v", access.Plan)
	}
}

func TestWebFigRuleIsNarrow(t *testing.T) {
	rule := webFigAddRule(webFigTestTunnel, "10.77.0.1", 80)
	for _, want := range []string{"chain=input", "action=accept", `in-interface="` + webFigTestTunnel + `"`, "src-address=10.77.0.1/32", "protocol=tcp", "dst-port=80", `comment="mega-magic:webfig"`} {
		if !strings.Contains(rule, want) {
			t.Fatalf("rule lacks %s: %s", want, rule)
		}
	}
}

func TestWebFigRefusesAnythingButATunnelAndASingleSource(t *testing.T) {
	r := &webFigScriptRunner{services: "mm|www|80|false|\n"}
	if _, err := readWebFigAccess(context.Background(), r, "ether1", "10.77.0.1/32"); err == nil {
		t.Fatal("non-tunnel interface accepted")
	}
	if _, err := readWebFigAccess(context.Background(), r, webFigTestTunnel, "10.77.0.0/24"); err == nil {
		t.Fatal("a whole network accepted as the source")
	}
}

func TestParseWebFigServicesTakesTheServiceNotItsConnections(t *testing.T) {
	rows := parseWebFigServices("noise\nmm|www|80|false|10.0.0.1/32;10.77.0.1\nmm|www|80|false|\nmm|www-ssl|bad|true|\n")
	if len(rows) != 1 || len(rows["www"].allowed) != 2 || !webFigAllows(rows["www"].allowed, "10.77.0.1") {
		t.Fatalf("%+v", rows)
	}
}
