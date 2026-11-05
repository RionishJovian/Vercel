# Burmese strings to review

Strings added since the IP/VLAN editors (2026-11-01). The Burmese is my own wording: please correct anything unnatural or wrong. Reply with the key and the corrected text, or edit the last column.

| Key | English | Burmese (to review) |
|---|---|---|
| `net.pendingTitle` | Waiting for you to keep this change | ဤပြောင်းလဲမှုကို ထားမည်လားဟု စောင့်နေသည် |
| `net.pendingBody` | The change is on the router and verified. Test a real client now. Unless you press Keep, the router puts the previous settings back by itself. | ပြောင်းလဲမှုသည် Router ပေါ်တွင်ရှိပြီး အတည်ပြုပြီးဖြစ်သည်။ ယခု တကယ့် client တစ်ခုဖြင့် စမ်းပါ။ Keep မနှိပ်ပါက Router က ယခင်ဆက်တင်များကို ကိုယ်တိုင် ပြန်ထားမည်။ |
| `net.timeLeft` | Time left | ကျန်ချိန် |
| `net.keep` | Keep these changes | ဤပြောင်းလဲမှုများကို ထားမည် |
| `net.revertNow` | Revert now | ယခု ပြန်ထားမည် |
| `net.kept` | Change kept. The restore timer was cancelled and checked. | ပြောင်းလဲမှုကို ထားပြီး။ ပြန်ထားမည့် timer ကို ရပ်ပြီး စစ်ပြီးဖြစ်သည်။ |
| `net.reverted` | Change reverted. The previous settings were restored and checked. | ပြောင်းလဲမှုကို ပြန်ထားပြီး။ ယခင်ဆက်တင်များကို ပြန်ထည့်ပြီး စစ်ပြီးဖြစ်သည်။ |
| `net.timeUp` | Time ran out, so the router put the previous settings back. Refresh to see its current state. | အချိန်ကုန်သွားသဖြင့် Router က ယခင်ဆက်တင်များကို ပြန်ထားလိုက်သည်။ လက်ရှိအခြေအနေ ကြည့်ရန် ပြန်ဖတ်ပါ။ |
| `net.heldApplied` | Applied and verified. Test a real client, then choose Keep or Revert. | သုံးပြီး အတည်ပြုပြီး။ တကယ့် client တစ်ခုဖြင့် စမ်းပြီး Keep သို့မဟုတ် Revert ရွေးပါ။ |
| `net.delete` | Delete this record | ဤမှတ်တမ်းကို ဖျက်မည် |
| `r.checking` | Checking… | စစ်နေသည်… |
| `r.noStatus` | Status unavailable | အခြေအနေ မရနိုင်ပါ |
| `r.hubSeen` | Last Magic Hub handshake | နောက်ဆုံး Magic Hub ချိတ်ဆက်ချိန် |
| `net.applying` | Backing up, applying and verifying router settings… | ဘက်ခ်အပ်လုပ်၊ သုံးပြီး Router ဆက်တင်များ စစ်နေသည်… |
| `net.verified` | Router settings verified. Backup: | Router ဆက်တင်များ အတည်ပြုပြီး။ ဘက်ခ်အပ်: |
| `net.testClient` | Test a real client before calling the service working. | ဝန်ဆောင်မှု အလုပ်လုပ်ပြီဟု မဆိုမီ တကယ့် client တစ်ခုဖြင့် စမ်းပါ။ |
| `net.applyFail` | Apply failed. | မအောင်မြင်ပါ။ |
| `net.rolledBack` | Previous settings were restored and verified. | ယခင်ဆက်တင်များကို ပြန်ထားပြီး အတည်ပြုပြီး။ |
| `net.rollbackUnconfirmed` | Rollback was not confirmed: inspect the router and its backup before trying again. | ပြန်ထားမှု မအတည်ပြုရသေးပါ - ထပ်မစမ်းမီ Router နှင့် ၎င်း၏ ဘက်ခ်အပ်ကို စစ်ပါ။ |
| `net.reloadHint` | Refresh status to edit again. | ထပ်ပြင်ရန် အခြေအနေ ပြန်ဖတ်ပါ။ |

## Magic Hub connection steps (2026-11-02)

| Key | English | Burmese (to review) |
|---|---|---|
| `hub.w.s1` | Prepare | ပြင်ဆင် |
| `hub.w.s2` | Connect | ချိတ် |
| `hub.w.s3` | Check | စစ် |
| `hub.w.t1` | 1. Prepare the router | ၁။ Router ကို ပြင်ဆင်ပါ |
| `hub.w.t2` | 2. Connect the router | ၂။ Router ကို ချိတ်ပါ |
| `hub.w.t3` | 3. Check the connection | ၃။ ချိတ်ဆက်မှုကို စစ်ပါ |
| `hub.w.where` | Where do I paste? | ဘယ်မှာ paste လုပ်ရမလဲ? |
| `hub.w.expect` | Paste the whole block and press Enter. Wait until it stops. Red text, or a line starting with STOP:, means it did not finish. | Block တစ်ခုလုံးကို paste လုပ်ပြီး Enter နှိပ်ပါ။ ရပ်သွားသည်အထိ စောင့်ပါ။ အနီရောင်စာ သို့မဟုတ် STOP: ဖြင့်စသော စာကြောင်း ပေါ်လာပါက မပြီးမြောက်ပါ။ |
| `hub.w.pasted` | I pasted it and saw no error | Paste လုပ်ပြီး error မမြင်ရပါ |
| `hub.w.once` | Paste this script once. If the router prints a line starting with STOP:, it refused to change anything. Do not run it again and again: copy that line and ask for help. | ဤ script ကို တစ်ကြိမ်သာ paste လုပ်ပါ။ Router က STOP: ဖြင့်စသော စာကြောင်း ပြပါက ဘာမှ မပြောင်းဘဲ ငြင်းလိုက်ခြင်း ဖြစ်သည်။ ထပ်ခါထပ်ခါ မ run ပါနှင့်။ ထိုစာကြောင်းကို ကူးပြီး အကူအညီ တောင်းပါ။ |
| `hub.w.help` | If the check does not pass, make sure that: | မအောင်မြင်ပါက အောက်ပါတို့ကို သေချာစစ်ပါ - |
| `hub.w.restart` | Start again from step 1 | အဆင့် ၁ မှ ပြန်စမည် |

The remaining `hub.w.*` keys (button labels, step bodies, the four checklist lines) follow the same wording; see `app/src/i18n/extra.ts`.

## Port speeds (2026-11-02)

| Key | English | Burmese (to review) |
|---|---|---|
| `r.measuring` | Measuring… | တိုင်းနေသည်… |

## Ports and health (2026-11-02)

| Key | English | Burmese (to review) |
|---|---|---|
| `r.updateAvail` | Update available | အပ်ဒိတ် ရနိုင်သည် |
| `r.noNewer` | No newer version known | ပိုသစ်သော ဗားရှင်း မသိရသေးပါ |
| `r.fwUpgrade` | Upgrade available | အဆင့်မြှင့်နိုင်သည် |

`r.full`, `r.half`, `r.software` and `r.firmware` are kept in English (Full duplex, Half duplex, RouterOS, Firmware) as technical terms.

## Firmware upgrade (2026-11-02)

| Key | English | Burmese (to review) |
|---|---|---|
| `fw.title` | Firmware upgrade | Firmware အဆင့်မြှင့်ခြင်း |
| `fw.held` | Firmware upgrades are switched off for now. | Firmware အဆင့်မြှင့်ခြင်းကို ယခုအချိန် ပိတ်ထားသည်။ |
| `fw.upgrade` | Upgrade firmware | Firmware အဆင့်မြှင့်မည် |
| `fw.steps` | What will happen | ဖြစ်မည့်အရာများ |
| `fw.before` | Before you continue | ဆက်မလုပ်မီ |
| `fw.understand` | I understand the router restarts and guests lose internet for a few minutes. | Router ပြန်စတင်မည်ဖြစ်ပြီး ဧည့်သည်များ မိနစ်အနည်းငယ် အင်တာနက် မရကြောင်း နားလည်ပါသည်။ |
| `fw.go` | Upgrade and restart | အဆင့်မြှင့်ပြီး ပြန်စတင်မည် |
| `fw.restarting` | Restarting. The router is offline for one to three minutes. Keep it powered on; this page updates by itself. | ပြန်စတင်နေသည်။ Router သည် တစ်မိနစ်မှ သုံးမိနစ်အထိ အော့ဖ်လိုင်း ဖြစ်မည်။ ပါဝါဖွင့်ထားပါ။ ဤစာမျက်နှာ ကိုယ်တိုင် ပြောင်းလဲမည်။ |
| `fw.done` | Done. The router reports firmware | ပြီးပါပြီ။ Router က firmware ကို ဤသို့ ပြသည် |
| `fw.timeout` | The router has not reported the new firmware after ten minutes. Check its power and cable, then refresh. | မိနစ် ၁၀ ကြာပြီးနောက်တွင်ပင် Router က firmware အသစ်ကို မပြသေးပါ။ ပါဝါနှင့် ကေဘယ်ကို စစ်ပြီး refresh လုပ်ပါ။ |

The other `fw.*` keys (preview-only note, checking, applying, failure texts) follow the same wording; see `app/src/i18n/extra.ts`.

## Port diagram (2026-11-02)

| Key | English | Burmese (to review) |
|---|---|---|
| `pp.hint` | Select a port on the picture to see its status. | ပုံပေါ်ရှိ port တစ်ခုကို ရွေးပြီး အခြေအနေ ကြည့်ပါ။ |
| `pp.unknown` | Not reported | မပြသေးပါ |

## Cable health (2026-11-02)

| Key | English | Burmese (to review) |
|---|---|---|
| `ch.attention` | needs attention | စစ်ဆေးသင့်သည် |
| `ch.neg` | The router could not finish agreeing a speed with the device on this port. Try another cable or restart the device. | Router သည် ဤ port ရှိ ကိရိယာနှင့် မြန်နှုန်း သဘောတူညီမှု မပြီးမြောက်ပါ။ ကေဘယ်အသစ် စမ်းကြည့်ပါ သို့မဟုတ် ကိရိယာကို ပြန်စတင်ပါ။ |
| `ch.half` | This port is running half duplex, which is slow and usually means a cable or settings problem. | ဤ port သည် half duplex ဖြင့် အလုပ်လုပ်နေသည်။ ၎င်းသည် နှေးပြီး ကေဘယ် သို့မဟုတ် ဆက်တင်ပြဿနာ ဖြစ်တတ်သည်။ |
| `ch.slowCable` | This link is slower than both ends support. The cable or a connector is the likely cause. | ဤလင့်ခ်သည် နှစ်ဖက်စလုံး ထောက်ပံ့နိုင်သည်ထက် နှေးနေသည်။ ကေဘယ် သို့မဟုတ် ချိတ်ဆက်ခေါင်း ပြဿနာ ဖြစ်နိုင်ခြေ အများဆုံး။ |
| `ch.partnerLimit` | Linked at the fastest speed the connected device offers. | ချိတ်ဆက်ထားသော ကိရိယာ ပေးနိုင်သည့် အမြန်ဆုံး မြန်နှုန်းဖြင့် ချိတ်ထားသည်။ |
| `ch.belowMax` | Linked below this port's top speed. The cable or the connected device may limit it. | ဤ port ၏ အမြန်ဆုံးထက် နှေးစွာ ချိတ်ထားသည်။ ကေဘယ် သို့မဟုတ် ချိတ်ထားသော ကိရိယာက ကန့်သတ်နေနိုင်သည်။ |
| `ch.fixedSpeed` | Auto-negotiation is off on this port, so its speed is set by hand. | ဤ port တွင် auto-negotiation ပိတ်ထားသဖြင့် မြန်နှုန်းကို လက်ဖြင့် သတ်မှတ်ထားသည်။ |
| `ch.errNow` | Receive errors are still increasing. A damaged cable or a loose connector is the likely cause. | လက်ခံမှု အမှားများ ဆက်တိုး နေသည်။ ကေဘယ် ပျက်စီးခြင်း သို့မဟုတ် ချိတ်ခေါင်း လျော့ခြင်း ဖြစ်နိုင်ခြေ အများဆုံး။ |
| `ch.errPast` | Errors were counted earlier but none in the last few minutes. | အမှားများကို ယခင်က ရေတွက်ထားသော်လည်း လွန်ခဲ့သော မိနစ်အနည်းငယ်အတွင်း မရှိပါ။ |
| `ch.errWatching` | Errors were counted since the router last started. Watching for new ones. | Router နောက်ဆုံး စတင်ပြီးကတည်းက အမှားများ ရေတွက်ထားသည်။ အသစ်များကို စောင့်ကြည့်နေသည်။ |

## Guest plan speed (2026-11-02)

| Key | English | Burmese (to review) |
|---|---|---|
| `g.plan.down` | Download speed (Mbps) | ဒေါင်းလုဒ် မြန်နှုန်း (Mbps) |
| `g.plan.up` | Upload speed (Mbps) | အပ်လုဒ် မြန်နှုန်း (Mbps) |
| `g.plan.speedHint` | Leave both at 0 for no speed limit. A speed applies to guests who buy this plan, from their next login. | မြန်နှုန်းကန့်သတ်ချက် မလိုပါက နှစ်ခုလုံး 0 ထားပါ။ ဤ plan ဝယ်သူများအတွက် နောက်တစ်ကြိမ် login မှစ၍ သက်ရောက်သည်။ |
| `g.plan.speedBad` | Plan speed needs both download and upload between 1 and 10000 Mbps, or both 0 for no limit. | Plan မြန်နှုန်းတွင် ဒေါင်းလုဒ်နှင့် အပ်လုဒ် နှစ်ခုလုံး Mbps ၁ မှ ၁၀၀၀၀ အတွင်း ဖြစ်ရမည်၊ မကန့်သတ်လိုပါက နှစ်ခုလုံး 0 ထားပါ။ |

## Device mode and PPPoE lock (2026-11-04)

| Key | English | Burmese (to review) |
|---|---|---|
| `cap.dmWarn` | Device mode turns off Hotspot or Scheduler, which vouchers need. In WinBox New Terminal run /system/device-mode/update hotspot=yes scheduler=yes, then within 5 minutes press the reset or mode button, or unplug the power. A normal reboot cancels the change. | Device mode က ဘောက်ချာအတွက် လိုအပ်သော Hotspot သို့မဟုတ် Scheduler ကို ပိတ်ထားသည်။ WinBox New Terminal တွင် /system/device-mode/update hotspot=yes scheduler=yes ကို ရိုက်ပြီး ၅ မိနစ်အတွင်း reset သို့မဟုတ် mode ခလုတ်ကို နှိပ်ပါ၊ သို့မဟုတ် ပါဝါကြိုးကို ဖြုတ်ပါ။ ပုံမှန် reboot လုပ်လျှင် ပြောင်းလဲမှု ပယ်ဖျက်သွားမည်။ |
| `bb.locked` | Paid PPPoE changes are switched off until they pass a real router trial. Saved records stay visible. | Router အမှန်ဖြင့် စမ်းသပ်အောင်မြင်သည်အထိ ငွေပေးချေ PPPoE ပြောင်းလဲမှုများကို ပိတ်ထားသည်။ သိမ်းထားသော မှတ်တမ်းများကို ကြည့်နိုင်သည်။ |
| `s.locked` | Too many failed sign-in attempts. Wait 15 minutes, then try again. | ဝင်ရောက်ရန် မအောင်မြင်မှု များလွန်းသည်။ ၁၅ မိနစ်စောင့်ပြီး ပြန်ကြိုးစားပါ။ |

## Network form labels (2026-11-04)

All Burmese text for the Internet Setup forms is in `app/src/i18n/networkLabels.ts`, next to its English wording. Please review that file as a whole.

## Console cleanup (2026-11-05)

| Key | English | Burmese (to review) |
|---|---|---|
| `c.pageFailed` | This page could not be shown | ဤစာမျက်နှာကို ပြ၍မရပါ |
| `c.pageFailedHelp` | Reload to try again. The tabs below still work. | ပြန်ဖွင့်ပြီး ထပ်စမ်းကြည့်ပါ။ အောက်ရှိ tab များ ဆက်သုံးနိုင်ပါသည်။ |
| `c.reload` | Reload | ပြန်ဖွင့်ရန် |
| `c.actionFailed` | That did not work. Try again. | မအောင်မြင်ပါ။ ထပ်စမ်းကြည့်ပါ။ |
| `r.mapTitle` | Network map | ကွန်ရက်မြေပုံ |
| `nw.firewallNote` | Existing firewall rules may need a separate review before guests can reach the internet. | ဧည့်သည်များ အင်တာနက်ရောက်မီ ရှိပြီးသား firewall စည်းမျဉ်းများကို သီးခြားစစ်ရနိုင်သည်။ |
| `more.title` | More | နောက်ထပ် |
| `v.ws.vouchers` | Codes | ကုဒ်များ |
| `v.ws.guest` | Internet guest page | အင်တာနက် ဧည့်သည် စာမျက်နှာ |
| `j.ws.vouchers` | Media vouchers | မီဒီယာ ဗောင်ချာများ |
| `j.ws.guest` | Media guest page | မီဒီယာ ဧည့်သည် စာမျက်နှာ |
| `dev.tm.title` | Team | အဖွဲ့ |
| `net.records` | records | မှတ်တမ်း |
## Magic Hub one paste (2026-11-05)

| Key | English | Burmese (to review) |
|---|---|---|
| `hub.w.scriptBody` | Copy the whole block into the WinBox terminal and press Enter. Mega Magic then connects by itself, so you can close this page. | Block တစ်ခုလုံးကို WinBox terminal ထဲ ကူးထည့်ပြီး Enter နှိပ်ပါ။ ပြီးလျှင် Mega Magic က အလိုအလျောက် ချိတ်ဆက်ပေးမည်ဖြစ်၍ ဤစာမျက်နှာကို ပိတ်နိုင်သည်။ |
| `hub.w.once` | If the router prints STOP:, fix what it names and paste the same script again. Pasting it again is always safe. | Router က STOP: ဟု ပြပါက ဖော်ပြထားသည့်အရာကို ပြင်ပြီး ဤ script ကိုပင် ထပ် paste လုပ်ပါ။ ထပ် paste လုပ်ခြင်းက အမြဲ ဘေးကင်းပါသည်။ |
| `hub.keepPrivate` | Keep this script private: it holds this router's tunnel key. | ဤ script ကို လျှို့ဝှက်ထားပါ။ ၎င်းတွင် ဤ Router ၏ tunnel key ပါဝင်သည်။ |
| `hub.delNoReset` | This ends its Magic Hub connection. The router's other settings stay as they are. | ၎င်းသည် Magic Hub ချိတ်ဆက်မှုကို ဖြတ်ပါမည်။ Router ၏ အခြားဆက်တင်များ မပြောင်းပါ။ |
| `hub.w.t3` | 3. Connected | ၃။ ချိတ်ဆက်ပြီးပါပြီ |
| `hub.w.s3` | Connected | ချိတ်ပြီး |
| `hub.w.waiting` | Waiting for your router… | သင့် Router ကို စောင့်နေသည်… |
| `hub.w.reached` | Your router reached Magic Hub. Finishing the connection… | သင့် Router သည် Magic Hub သို့ ရောက်ပါပြီ။ ချိတ်ဆက်မှုကို အပြီးသတ်နေသည်… |
| `hub.connected` | Connected to Magic Hub | Magic Hub နှင့် ချိတ်ဆက်ပြီး |
| `hub.delDoneTitle` | Record deleted | မှတ်တမ်းကို ဖျက်ပြီးပါပြီ |
| `hub.delDoneBody` | Paste this in WinBox to remove the Magic Hub setup from the router. Skip it if you will connect this router again. | Router မှ Magic Hub setup ကို ဖယ်ရှားရန် ဤ script ကို WinBox တွင် paste လုပ်ပါ။ ဤ Router ကို ပြန်ချိတ်မည်ဆိုပါက ကျော်နိုင်သည်။ |
| `hub.delDone` | Done | ပြီးပါပြီ |
| `hub.progress.expired` | Setup expired. Open Connect Magic Hub for a new script. | Setup သက်တမ်းကုန်သွားပါပြီ။ Script အသစ်အတွက် Magic Hub ချိတ်မည် ကို ဖွင့်ပါ။ |
| `hub.progress.pending` | Setup is waiting for the router to connect. | Setup သည် Router ချိတ်ဆက်လာရန် စောင့်နေသည်။ |
| `hub.delBody` | Telemetry, backups and change history may be deleted with it. If it has voucher or PPPoE history, the record and that history stay, and only its connection ends. | တယ်လီမီထရီ၊ ဘက်ခ်အပ်နှင့် ပြောင်းလဲမှုမှတ်တမ်းများလည်း အတူ ပျက်နိုင်သည်။ ဗောင်ချာ သို့မဟုတ် PPPoE မှတ်တမ်းရှိပါက မှတ်တမ်းနှင့် ထိုမှတ်တမ်းများ ဆက်ရှိပြီး ချိတ်ဆက်မှုသာ ဖြတ်ပါမည်။ |
| `hub.keptTitle` | Connection ended. History kept. | ချိတ်ဆက်မှု ဖြတ်ပြီးပါပြီ။ မှတ်တမ်းများ ဆက်ရှိပါသည်။ |
| `hub.keptBody` | This router's voucher, PPPoE and sales history stay on its record. To connect this router or a replacement, press Connect Magic Hub and paste the new script. If you are retiring it, paste this cleanup script in WinBox. | ဤ Router ၏ ဗောင်ချာ၊ PPPoE နှင့် အရောင်းမှတ်တမ်းများ ၎င်း၏မှတ်တမ်းတွင် ဆက်ရှိပါသည်။ ဤ Router သို့မဟုတ် အစားထိုး Router ကို ချိတ်ရန် Magic Hub ချိတ်မည် ကိုနှိပ်ပြီး script အသစ်ကို paste လုပ်ပါ။ အသုံးမပြုတော့ပါက ဤ cleanup script ကို WinBox တွင် paste လုပ်ပါ။ |
| `hub.keptNote` | Connection ended. This router's voucher, PPPoE and sales history stay on its record. To connect this router or a replacement, press Connect Magic Hub and paste the new script. | ချိတ်ဆက်မှု ဖြတ်ပြီးပါပြီ။ ဤ Router ၏ ဗောင်ချာ၊ PPPoE နှင့် အရောင်းမှတ်တမ်းများ ၎င်း၏မှတ်တမ်းတွင် ဆက်ရှိပါသည်။ ဤ Router သို့မဟုတ် အစားထိုး Router ကို ချိတ်ရန် Magic Hub ချိတ်မည် ကိုနှိပ်ပြီး script အသစ်ကို paste လုပ်ပါ။ |

## Vouchers table, Live users and Print layouts (2026-11-05)

Keys are in `app/src/i18n/tables.ts`. "စလစ်" (slip), "ပေးအပ်မည်" (hand out), "ဖြုတ်မည်" (kick) and "မလှုပ်ရှား" (idle) are the wordings I am least sure of.

| Key | English | Burmese (to review) |
|---|---|---|
| `vt.search` | Search code or plan | ကုဒ် သို့မဟုတ် ပလန် ရှာရန် |
| `vt.allBatches` | All batches | အစုအားလုံး |
| `vt.all` | All | အားလုံး |
| `vt.status.unused` | Unused | မသုံးရသေး |
| `vt.status.active` | Active | သုံးနေသည် |
| `vt.status.used` | Used | သုံးပြီး |
| `vt.status.expired` | Expired | သက်တမ်းကုန် |
| `vt.status.revoked` | Revoked | ပယ်ဖျက်ပြီး |
| `vt.status.banned` | Banned | ပိတ်ပင်ထား |
| `vt.code` | Code | ကုဒ် |
| `vt.plan` | Plan / batch | ပလန် / အစု |
| `vt.statusCol` | Status | အခြေအနေ |
| `vt.actions` | Actions | လုပ်ဆောင်ချက် |
| `vt.slip` | Slip | စလစ် |
| `vt.handOut` | Hand out | ပေးအပ်မည် |
| `vt.delete` | Delete | ဖျက်မည် |
| `vt.selectPage` | Select this page | ဤစာမျက်နှာကို ရွေးမည် |
| `vt.selectRow` | Select | ရွေးမည် |
| `vt.selected` | {n} selected | {n} ခု ရွေးထား |
| `vt.selectAll` | Select all {n} | {n} ခုလုံး ရွေးမည် |
| `vt.clear` | Clear | ရှင်းမည် |
| `vt.slips` | Slips ({n}) | စလစ် ({n}) |
| `vt.deleteN` | Delete ({n}) | ဖျက်မည် ({n}) |
| `vt.slipOff` | Only unused codes verified on the router, in a finished batch, can be printed. | ပြီးဆုံးသော အစုထဲမှ Router တွင် အတည်ပြုပြီး မသုံးရသေးသော ကုဒ်ကိုသာ ပရင့်ထုတ်နိုင်သည်။ |
| `vt.notInstalled` | Not on router | Router ထဲ မရှိသေး |
| `vt.hiddenCode` | Hidden until verified on the router | Router တွင် အတည်ပြုသည်အထိ ဖုံးထားသည် |
| `vt.deleteConfirm` | Delete {n} voucher(s)? This cannot be undone. | ဗောင်ချာ {n} ခု ဖျက်မလား? ပြန်ယူ၍ မရပါ။ |
| `vt.deleted` | {ok} of {n} deleted. | {n} ခုအနက် {ok} ခု ဖျက်ပြီး။ |
| `vt.noMatch` | No voucher matches. | ကိုက်ညီသော ဗောင်ချာ မရှိပါ။ |
| `vt.pageOf` | Page {p} · {a}–{b} of {n} | စာမျက်နှာ {p} · {n} ခုအနက် {a}–{b} |
| `vt.page` | Page | စာမျက်နှာ |
| `vt.prev` | Previous | ရှေ့သို့ |
| `vt.next` | Next | နောက်သို့ |
| `vt.printBatch` | Print this batch | ဤအစုကို ပရင့်ထုတ်မည် |
| `vt.refreshed` | Usage read from the router. | Router မှ အသုံးပြုမှု ဖတ်ပြီး။ |
| `lu.all` | All | အားလုံး |
| `lu.active` | Active | သုံးနေသည် |
| `lu.idle` | Idle | မလှုပ်ရှား |
| `lu.near` | Near expiry | သက်တမ်းကုန်ခါနီး |
| `lu.over` | Over quota | ဒေတာကျော် |
| `lu.banned` | Banned MAC | ပိတ်ထားသော MAC |
| `lu.updated` | Updated | ဖတ်ချိန် |
| `lu.status` | Status | အခြေအနေ |
| `lu.voucher` | Voucher | ဗောင်ချာ |
| `lu.ip` | IP | IP |
| `lu.mac` | MAC | MAC |
| `lu.uptime` | Uptime | သုံးချိန် |
| `lu.left` | Left | ကျန်ချိန် |
| `lu.downUp` | Down / Up | ဒေါင်း / အပ် |
| `lu.kick` | Kick | ဖြုတ်မည် |
| `lu.ban` | Ban MAC | MAC ပိတ်မည် |
| `lu.rate` | Rate | နှုန်း |
| `lu.copy` | Copy | ကူးမည် |
| `lu.copied` | Copied. | ကူးပြီး။ |
| `lu.copyFailed` | Could not copy. | မကူးနိုင်ပါ။ |
| `lu.kickN` | Kick ({n}) | ဖြုတ်မည် ({n}) |
| `lu.copyN` | Copy ({n}) | ကူးမည် ({n}) |
| `lu.kickConfirm` | Disconnect {n} guests? They can reconnect with valid access. | ဧည့်သည် {n} ဦးကို ဖြတ်ချမလား? သက်တမ်းရှိလျှင် ပြန်ချိတ်နိုင်သည်။ |
| `lu.kicked` | {ok} of {n} disconnected and confirmed by the router. | {n} ဦးအနက် {ok} ဦး ဖြတ်ပြီး Router က အတည်ပြုပြီး။ |
| `lu.selectAll` | Select all shown | ပြထားသမျှ ရွေးမည် |
| `lu.notRead` | Not read from the router yet | Router မှ မဖတ်ရသေးပါ |
| `pl.title` | Voucher print layouts | ဗောင်ချာ ပရင့်ပုံစံများ |
| `pl.lead` | Configure the counter receipt only. These settings do not change the guest portal or the RouterOS Hotspot. | ကောင်တာ ပြေစာကိုသာ ပြင်ဆင်သည်။ ဤဆက်တင်များက ဧည့်သည် portal နှင့် RouterOS Hotspot ကို မပြောင်းလဲပါ။ |
| `pl.back` | Back to vouchers | ဗောင်ချာများသို့ ပြန်မည် |
| `pl.card` | Default counter receipt | ပုံသေ ကောင်တာ ပြေစာ |
| `pl.cardLead` | Applied to every voucher slip and batch print. | ဗောင်ချာ စလစ်တိုင်းနှင့် အစုလိုက် ပရင့်တိုင်းတွင် သုံးသည်။ |
| `pl.paper` | Thermal paper width | Thermal စက္ကူ အကျယ် |
| `pl.p58` | 58 mm (small printer) | ၅၈ မီလီမီတာ (ပရင်တာ အသေး) |
| `pl.p80` | 80 mm (standard counter printer) | ၈၀ မီလီမီတာ (ကောင်တာ ပရင်တာ ပုံမှန်) |
| `pl.a4` | A4 sheet | A4 စာရွက် |
| `pl.other` | {n} mm | {n} မီလီမီတာ |
| `pl.terms` | Voucher use terms | ဗောင်ချာ အသုံးပြုမှု စည်းကမ်း |
| `pl.qr` | Include QR code | QR ကုဒ် ထည့်မည် |
| `pl.showExpiry` | Show expiry / use terms | သက်တမ်း / စည်းကမ်း ပြမည် |
| `pl.test` | Print test receipt | စမ်းသပ် ပြေစာ ပရင့်ထုတ်မည် |
| `pl.preview` | Live preview | တိုက်ရိုက် နမူနာ |
| `pl.previewLead` | The browser print dialog controls the physical printer. The QR code holds the voucher code. | ပရင်တာကို browser ၏ ပရင့် dialog က ထိန်းချုပ်သည်။ QR ကုဒ်တွင် ဗောင်ချာကုဒ် ပါသည်။ |
| `pl.demoPlan` | Demo plan | နမူနာ ပလန် |
| `pl.reload` | Reload | ပြန်ဖတ်မည် |

### Added with the router actions

| Key | English | Burmese (to review) |
|---|---|---|
| `vt.handOutOff` | Hand out is not available yet: its records need a database update. | ပေးအပ်မည် ကို မသုံးနိုင်သေးပါ - ၎င်း၏ မှတ်တမ်းအတွက် database အပ်ဒိတ် လိုသည်။ |
| `vt.deleteOff` | Delete is not available yet for codes on the router: this router action has not passed acceptance. | Router ပေါ်ရှိ ကုဒ်များကို မဖျက်နိုင်သေးပါ - ဤ Router လုပ်ဆောင်ချက်ကို စမ်းသပ်အတည်ပြုခြင်း မပြီးသေးပါ။ |
| `vt.deleteUnused` | Only an unused code in a finished batch can be deleted. | ပြီးဆုံးသော အစုထဲမှ မသုံးရသေးသော ကုဒ်ကိုသာ ဖျက်နိုင်သည်။ |
| `vt.deleteRouterConfirm` | Delete {n} code(s)? They stop working on the router and stay on record as Revoked. This cannot be undone. | ကုဒ် {n} ခု ဖျက်မလား? Router တွင် အလုပ်မလုပ်တော့ဘဲ ပယ်ဖျက်ပြီး အဖြစ် မှတ်တမ်းတွင် ကျန်မည်။ ပြန်ယူ၍ မရပါ။ |
| `vt.handOutN` | Hand out ({n}) | ပေးအပ်မည် ({n}) |
| `vt.handedOut` | Handed out | ပေးအပ်ပြီး |
| `vt.handOutFailed` | Some codes could not be marked. Check them and try again. | ကုဒ်အချို့ကို မမှတ်နိုင်ပါ။ စစ်ပြီး ထပ်ကြိုးစားပါ။ |
| `lu.kickOff` | Kick and Ban MAC are not available yet: these router actions have not passed acceptance. | ဖြုတ်မည် နှင့် MAC ပိတ်မည် ကို မသုံးနိုင်သေးပါ - ဤ Router လုပ်ဆောင်ချက်များကို စမ်းသပ်အတည်ပြုခြင်း မပြီးသေးပါ။ |
| `lu.off` | Rate is not available: there is no reliable way yet to change the speed of a guest already online. | နှုန်း ကို မသုံးနိုင်ပါ - အွန်လိုင်းရှိပြီးသား ဧည့်သည်၏ အမြန်နှုန်းကို ယုံကြည်စိတ်ချစွာ ပြောင်းရန် နည်းလမ်း မရှိသေးပါ။ |
| `lu.banConfirm` | Block {mac}? This device loses internet until you remove the block. | {mac} ကို ပိတ်မလား? ပိတ်ခြင်းကို မဖယ်မချင်း ဤစက်တွင် အင်တာနက် မရပါ။ |
| `lu.unbanConfirm` | Remove the block on {mac}? | {mac} ၏ ပိတ်ထားခြင်းကို ဖယ်မလား? |
| `lu.bannedDone` | Device blocked and confirmed by the router. | စက်ကို ပိတ်ပြီး Router က အတည်ပြုပြီး။ |
| `lu.unbanned` | Block removed and confirmed by the router. The device can take about a minute to get back. | ပိတ်ထားခြင်းကို ဖယ်ပြီး Router က အတည်ပြုပြီး။ စက် ပြန်ရောက်ရန် တစ်မိနစ်ခန့် ကြာနိုင်သည်။ |
| `lu.banFailed` | Could not change the block. | ပိတ်ထားခြင်းကို မပြောင်းနိုင်ပါ။ |
| `lu.noBans` | No device is blocked. | ပိတ်ထားသော စက် မရှိပါ။ |
| `lu.banNote` | Note | မှတ်ချက် |
| `lu.banByApp` | Blocked from this app | ဤ app မှ ပိတ်ထားသည် |
| `lu.banOther` | Set on the router, not by this app | ဤ app မဟုတ်ဘဲ Router တွင် သတ်မှတ်ထားသည် |
| `lu.unban` | Remove block | ပိတ်ခြင်း ဖယ်မည် |

Printed on the receipt itself, in `printCodes.ts` (Burmese and English together, because guests read it):

| English | Burmese (to review) |
|---|---|
| Plan | ပလန် |
| Price | စျေးနှုန်း |
| Valid | သက်တမ်း |
| Support | ဆက်သွယ်ရန် |
