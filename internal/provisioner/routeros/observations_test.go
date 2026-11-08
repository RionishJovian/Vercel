package routeros

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestObservationsFixedReadOnlyMenusAndPartialFailure(t *testing.T) {
	calls := 0
	r := commandRunnerFunc(func(_ context.Context, cmd string) (string, error) {
		calls++
		for _, bad := range []string{" add ", " set ", " remove ", " disable ", " enable ", "password", "secret", "reset-counters"} {
			if strings.Contains(cmd, bad) {
				t.Fatalf("unsafe observation: %s", cmd)
			}
		}
		if strings.Contains(cmd, "/ip arp") {
			return "id: *A\naddress: 10.0.0.2\nmac-address: 02:00:00:00:00:01\ninterface: bridge\n", nil
		}
		return "", fmt.Errorf("source unavailable")
	})
	result, e := readObservations(context.Background(), r)
	if e != nil || calls != len(observationOrder)+1 || !result.Available["arp"] || result.Available["hotspot_sessions"] || len(result.Objects["arp"]) != 1 || result.WriteEnabled || result.ObservedAt == "" {
		t.Fatalf("%+v %v", result, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls = 0
	if _, e = readObservations(ctx, r); e == nil || calls != 0 {
		t.Fatal("cancellation not respected")
	}
}

func TestCAPsMANReadOnlySourcesDoNotInventConnectedAPs(t *testing.T) {
	modern := map[string]string{
		"capsman_modern_caps": "identity: lobby-cap\naddress: 10.20.0.4\nstate: Run\nradios: 1\n",
		"capsman_modern_profiles": "name: guest\nssid: Shop WiFi\nmode: ap\ndisabled: false\n",
	}
	// Construct fixtures from declared fields so RouterOS parser contracts
	// and menu names cannot accidentally diverge.
	observed := map[string]bool{}
	r := commandRunnerFunc(func(_ context.Context, cmd string) (string, error) {
		if strings.Contains(cmd," provision ") || strings.Contains(cmd," password") ||
			strings.Contains(cmd," private-key") || strings.Contains(cmd," security.") {
			t.Fatalf("unsafe CAPsMAN observation command: %q",cmd)
		}
		for name := range modern {
			menu := observationMenus[name]
			if cmd == networkReadCommand(menu) {
				observed[name]=true
				var values []string
				for _,field:=range menu.fields {
					value:=""
					if field=="identity"{value="lobby-cap"}
					if field=="address"{value="10.20.0.4"}
					if field=="state"{value="Run"}
					if field=="radios"{value="1"}
					if field=="name"{value="guest"}
					if field=="ssid"{value="Shop WiFi"}
					if field=="mode"{value="ap"}
					if field=="disabled"{value="false"}
					values=append(values,field+": "+value)
				}
				return "id: *1\n"+strings.Join(values,"\n")+"\n",nil
			}
		}
		return "",fmt.Errorf("controller menu not installed")
	})
	result,err:=readObservations(context.Background(),r)
	if err!=nil{t.Fatal(err)}
	for name:=range modern {
		if !observed[name] || !result.Available[name] || len(result.Objects[name])!=1 {
			t.Fatalf("expected observed modern CAPsMAN source %s: %+v",name,result)
		}
	}
	for _,name:=range []string{"capsman_legacy_caps","capsman_legacy_radios","capsman_legacy_profiles"} {
		if result.Available[name] || len(result.Objects[name])!=0 {
			t.Fatalf("missing legacy menu misreported as an empty available AP list: %s",name)
		}
	}
	if result.WiFiWriteEnabled || result.WriteEnabled {t.Fatal("CAPsMAN inventory enabled writes")}
}

func TestCAPsMANObservationMenusExcludeSecretsAndWrites(t *testing.T) {
	for _,name:=range []string{
		"capsman_modern_caps","capsman_modern_radios","capsman_modern_profiles",
		"capsman_legacy_caps","capsman_legacy_radios","capsman_legacy_profiles",
	} {
		menu,ok:=observationMenus[name]
		if !ok{t.Fatalf("missing %s",name)}
		cmd:=networkReadCommand(menu)
		for _,bad:=range []string{" password"," passphrase"," security."," add "," set "," remove "," provision "," reset-configuration"} {
			if strings.Contains(cmd,bad) {t.Fatalf("%s leaked config/write operation: %s",name,cmd)}
		}
		if !strings.Contains(cmd," find]")||!strings.Contains(cmd," get $id "){
			t.Fatalf("%s is not a read-only object listing: %s",name,cmd)
		}
	}
}
