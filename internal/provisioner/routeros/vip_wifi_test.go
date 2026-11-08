package routeros

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type vipTestRunner struct {
	users map[string]string
	profiles map[string]string
	commands []string
	foreign bool
}

func (r *vipTestRunner) Run(_ context.Context, cmd string) (string,error) {
	r.commands=append(r.commands,cmd)
	switch {
	case cmd=="/ip hotspot print count-only where disabled=no": return "1",nil
	case cmd=="/ip hotspot print value-list detail where disabled=no": return "name: hs1\n",nil
	case strings.HasPrefix(cmd,"/ip hotspot user profile print count-only where name="):
		return guardCount(r.profiles,cmd),nil
	case strings.HasPrefix(cmd,"/ip hotspot user profile print value-list detail where name="):
		return guardBody(r.profiles,cmd),nil
	case strings.HasPrefix(cmd,"/ip hotspot user profile add name="):
		return "",guardAdd(r.profiles,cmd,"profile")
	case strings.HasPrefix(cmd,"/ip hotspot user print value-list detail where name="):
		return guardBody(r.users,cmd),nil
	case strings.HasPrefix(cmd,"/ip hotspot user add name="):
		args:=guardAddArgs(cmd)
		name:=args["name"]
		if name=="" || r.users[name]!=""{return "",errors.New("duplicate or missing name")}
		r.users[name]="name: "+name+"\nserver: "+args["server"]+"\nprofile: "+args["profile"]+"\ncomment: "+args["comment"]+"\n"
		return "",nil
	case strings.HasPrefix(cmd,"/ip hotspot user disable [find where name="):
		name:=vipTestFindName(cmd)
		if r.users[name]==""{return "",errors.New("not found")}
		r.users[name]+="disabled: yes\n"
		return "",nil
	case strings.HasPrefix(cmd,"/ip hotspot active remove [find where user="): return "",nil
	case strings.HasPrefix(cmd,"/ip hotspot active print count-only where user="): return "0",nil
	default: return "",errors.New("unexpected router command")
	}
}
func vipTestFindName(cmd string)string {
	i:=strings.Index(cmd,"name=")
	if i<0{return ""}
	rest:=cmd[i+5:]
	if k:=strings.Index(rest," ");k>=0{rest=rest[:k]}
	return strings.Trim(rest,"\"")
}
func vipFixture() VIPWiFiCode {
	return VIPWiFiCode{ID:"11111111-1111-4111-8111-111111111111",Code:"VIP-ABCDEFGH",MaxDevices:3}
}
func TestVIPWiFiCreatesOnlySeparateProfileAndVerifies(t *testing.T) {
	r:=&vipTestRunner{users:map[string]string{},profiles:map[string]string{}}
	v:=vipFixture()
	if err:=ensureVIPWiFiCode(context.Background(),r,v);err!=nil{t.Fatal(err)}
	if !strings.Contains(r.profiles[vipProfileName(3)],"shared-users: 3") {
		t.Fatal("multi-device profile not verified")
	}
	for _,cmd:=range r.commands{
		if strings.Contains(cmd,"mega-magic-voucher-v3") || strings.Contains(cmd,"shared-users=1"){
			t.Fatalf("VIP must not modify ordinary voucher profile: %s",cmd)
		}
	}
	before:=len(r.commands)
	if err:=ensureVIPWiFiCode(context.Background(),r,v);err!=nil{t.Fatalf("retry: %v",err)}
	for _,cmd:=range r.commands[before:] {
		if strings.Contains(cmd," add "){t.Fatal("idempotent retry created a duplicate object")}
	}
	if err:=revokeVIPWiFiCode(context.Background(),r,v);err!=nil{t.Fatal(err)}
	if !strings.Contains(r.users[v.Code],"disabled: yes"){t.Fatal("revoked user not disabled")}
}
func TestVIPWiFiRejectsPolicyAndForeignObject(t *testing.T) {
	for _,bad:=range []VIPWiFiCode{
		{ID:"11111111-1111-4111-8111-111111111111",Code:"VIP-ABCDEFGH",MaxDevices:1},
		{ID:"11111111-1111-4111-8111-111111111111",Code:"VIP-ABCDEFGH",MaxDevices:9},
		{ID:"bad",Code:"VIP-ABCDEFGH",MaxDevices:3},
		{ID:"11111111-1111-4111-8111-111111111111",Code:"VIP-ABCDEFG0",MaxDevices:3},
	}{
		r:=&vipTestRunner{users:map[string]string{},profiles:map[string]string{}}
		if err:=ensureVIPWiFiCode(context.Background(),r,bad);err==nil || len(r.commands)>0{
			t.Fatalf("invalid VIP policy caused router commands: %+v err=%v",bad,err)
		}
	}
	v:=vipFixture()
	r:=&vipTestRunner{users:map[string]string{},profiles:map[string]string{
		vipProfileName(3):"name: "+vipProfileName(3)+"\nshared-users: 8\n",
	}}
	if err:=ensureVIPWiFiCode(context.Background(),r,v);err==nil {
		t.Fatal("foreign shared-users profile was adopted")
	}
	for _,cmd:=range r.commands{
		if strings.HasPrefix(cmd,"/ip hotspot user add"){t.Fatal("user created against foreign profile")}
	}
}
func TestVIPWiFiForeignAccountCannotBeRevoked(t *testing.T){
	v:=vipFixture()
	r:=&vipTestRunner{users:map[string]string{
		v.Code:"name: "+v.Code+"\ncomment: someone-else\nprofile: "+vipProfileName(v.MaxDevices)+"\n",
	},profiles:map[string]string{}}
	if err:=revokeVIPWiFiCode(context.Background(),r,v);err==nil {t.Fatal("foreign login revoked")}
	for _,cmd:=range r.commands{
		if strings.Contains(cmd," disable ") || strings.Contains(cmd,"active remove"){
			t.Fatal("foreign account received write command")
		}
	}
}
