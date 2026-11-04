package routeros

// Script rules that keep these sources correct on a real router:
//   - A local named like a RouterOS property reads the record's property inside
//     `where`, so `where name=$name` matches every record. Use bb-prefixed names.
//   - bbClockSource is also sent inside an SSH command, where multi-line input
//     fails before RouterOS 7.25 and each CLI line has its own scope. Keep it on
//     one line.
//   - `:toarray` element types are undocumented. Compare through :tonum or :tostr.

// One volatile watermark per script owner avoids frequent flash writes. After
// reboot (or a new execution context), trusted NTP must establish the clock.
// Once established, losing the central app or an NTP server does not open access.
const bbClockSource = `:local now ([:tonsec [:timestamp]] / 1000000000); :global mmBBClockHigh; :local clockOK true; ` +
	`:if ([:typeof $now] != "num" or $now < 1000000000 or $now > 4102444800) do={ :set clockOK false; } else={ ` +
	`:if ([:typeof $mmBBClockHigh] != "num") do={ ` +
	`:local ntp "unknown"; :do { :set ntp [/system ntp client get status]; } on-error={}; ` +
	`:if ($ntp != "synchronized") do={ :set clockOK false; } else={ :set mmBBClockHigh $now; }; ` +
	`} else={ :if ($now < $mmBBClockHigh) do={ :set clockOK false; } else={ :set mmBBClockHigh $now; }; }; ` +
	`}; `

// u/bbUser/p identify one owned secret/profile. Never changes first-use evidence
// or operation identity. The independent profile anchor survives renewals. The
// disabled flag is written only when it changes: this runs every 15 seconds and
// the secret is stored on flash.
const bbEnforceOneSource = `:local raw [/ppp secret get $u comment]; :local a [:toarray $raw]; :local deny false;
 :if ([:len $a] != 8 or $clockOK = false) do={ :set deny true; } else={
  :local first [:tonum ($a->4)]; :local expiry [:tonum ($a->5)]; :local period [:tonum ($a->3)]; :local paused [:tonum ($a->6)]; :local seen [:tonum ($a->7)];
  :local anchor [:toarray [/ppp profile get $p comment]];
  :if ([:len $anchor] = 2 and [:tostr ($anchor->0)] = "mega-magic:broadband:v1") do={
   :local ft [:tonum ($anchor->1)];
   :if ([:typeof $ft] != "num" or $ft < 1000000000 or $ft > $now or ($first > 0 and $first != $ft)) do={ :set deny true; } else={
    :if ($first = 0) do={ :set first $ft; :set expiry ($ft + $period); };
   };
  } else={ :if ([:tostr $anchor] != "mega-magic:broadband:v1" or $first != 0 or [:len [/ppp active find where name=$bbUser service=pppoe]] > 0) do={ :set deny true; }; };
  :if ([:typeof $first] != "num" or [:typeof $expiry] != "num" or [:typeof $period] != "num" or [:typeof $paused] != "num" or [:typeof $seen] != "num") do={ :set deny true; } else={
   :if ($now < $seen or $paused != 0 or $period < 86400 or $period > 316224000 or ($first > 0 and $now >= $expiry)) do={ :set deny true; };
  };
 };
 :local off [:tostr [/ppp secret get $u disabled]];
 :if ($deny) do={ :if ($off != "true" and $off != "yes") do={ /ppp secret set $u disabled=yes; }; /ppp active remove [find where name=$bbUser service=pppoe]; } else={ :if ($off = "true" or $off = "yes") do={ /ppp secret set [find where name=$bbUser comment=$raw profile=$p service=pppoe] disabled=no; }; }; `

const bbGuardSource = bbClockSource + `:foreach u in=[/ppp secret find where comment~"^mm-bb-v1,"] do={
 :local bbUser [/ppp secret get $u name]; :local p [/ppp secret get $u profile];
 :if ([:pick $p 0 6] != "mm-bb-") do={ /ppp secret set $u disabled=yes; /ppp active remove [find where name=$bbUser service=pppoe]; } else={ :do { ` + bbEnforceOneSource + ` } on-error={ /ppp secret set $u disabled=yes; /ppp active remove [find where name=$bbUser service=pppoe]; }; }; }`

// The hook nests its branches: the manual documents :return only for functions.
func bbOnUp(name, id string) string {
	return `:local bbUser ` + routerOSQuoted(name) + `; :if ($user != $bbUser) do={ /ppp active remove [find where name=$user service=pppoe]; } else={ ` +
		`:local u [/ppp secret find where name=$bbUser]; :if ([:len $u] = 1) do={ :local p [/ppp secret get $u profile]; ` +
		`:if ($p != ` + routerOSQuoted(bbProfile(id)) + `) do={ /ppp active remove [find where name=$bbUser service=pppoe]; } else={ ` + bbClockSource +
		`:if ($clockOK and [/ppp profile get $p comment] = "mega-magic:broadband:v1") do={ /ppp profile set $p comment=("mega-magic:broadband:v1," . $now); }; ` +
		bbEnforceOneSource + `}; }; }`
}
