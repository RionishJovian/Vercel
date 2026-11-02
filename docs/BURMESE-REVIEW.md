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
