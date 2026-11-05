# Router trial for guest actions

Kick, Ban MAC, removing a block and Delete for a code on the router all change a live router. They stay refused until this trial passes on a real router and the switch is set.

The switch is off unless both of these are set:

```
MM_NETWORK_CONTROLS_ENABLED=true
MM_GUEST_ACTIONS_ENABLED=true
```

Run on the lab router with one phone, using a voucher made for the test. Do not use a customer's router.

## Reads

| Step | Pass when |
|---|---|
| Open Live users with a guest online | The session list loads. Idle and Banned MAC show numbers, not a dash. |
| Leave the phone idle for 6 minutes, then refresh | The row shows Idle. |
| Check Left against WinBox | It matches `session-time-left`, or the app's voucher expiry if that is sooner. |

## Kick

| Step | Pass when |
|---|---|
| Kick the phone | The app says the router confirmed it, and WinBox no longer lists the session. |
| Browse on the phone | It gets the login page, not a silent login. The forum and the manual disagree on this. |
| Log in with the same code | It works, and time and data carry on from before. |
| Keep a second page open, kick from the first, then kick from the second | The second says the session changed. Nothing else is removed. |

## Ban MAC

| Step | Pass when |
|---|---|
| Ban the phone | The binding is listed first, `type=blocked`, comment `mega-magic:ban`. The session has ended. |
| Browse on the phone | No internet and no login page. |
| Remove the block from the Banned MAC chip | The binding is gone and the phone can log in again. |
| Add your own binding for that MAC in WinBox, then Ban from the app | The app refuses and your binding is untouched. |
| Repeat with the phone's private Wi-Fi address on | Record what happens: a new random MAC gets round a ban. |

## Delete a code on the router

| Step | Pass when |
|---|---|
| Delete an unused code | The user is gone from `/ip hotspot user`, and the voucher shows Revoked. |
| Try that code on the phone | Login is refused. |
| Delete a code that has been used, or is online | The app refuses, and the user stays. |
| Run Refresh usage afterwards | The sync still succeeds. |
| Print the batch afterwards | The deleted code is not printed. |

## Not built

Rate stays unavailable. The forum shows no reliable way to change the speed of a guest already online. Look at this again only with a router trial of the session's dynamic queue.
