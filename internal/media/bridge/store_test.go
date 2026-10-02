package bridge

import (
	"encoding/json"
	"testing"
)

func TestSecretsAndOperationBoundary(t *testing.T) {
	a, e := Secret()
	if e != nil {
		t.Fatal(e)
	}
	b, _ := Secret()
	if a == b || len(a) != 64 {
		t.Fatal("unsafe entropy")
	}
	hash, e := Hash(a)
	if e != nil || hash == a {
		t.Fatal("unsafe secret storage")
	}
	if _, e = Hash("bad"); e == nil {
		t.Fatal("accepted bad secret")
	}
	for _, op := range []string{"shell", "delete_library", "reboot"} {
		if ValidateOperation(op, json.RawMessage(`{}`)) == nil {
			t.Fatal("arbitrary operation")
		}
	}
	if ValidateOperation("scan", json.RawMessage(`{"confirm":true}`)) != nil {
		t.Fatal("scan validation")
	}
	if ValidateOperation("scan", json.RawMessage(`{}`)) == nil {
		t.Fatal("missing confirmation")
	}
	if ValidateOperation("add_library", json.RawMessage(`{"confirm":true,"library":{"Name":"Videos","Kind":"homevideos","Paths":["relative"]}}`)) == nil {
		t.Fatal("unsafe folders")
	}
}
