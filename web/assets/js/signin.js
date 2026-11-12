/* Sign in and create account. Same endpoints and behaviour as before (/auth/login, /auth/register, then the console);
   adds inline validation, a busy state, a show-password toggle, a Caps Lock hint and a safe return path. */
(function () {
  var alertBox = document.getElementById("alert");
  var loginForm = document.getElementById("login-form");
  var registerForm = document.getElementById("register-form");
  var title = document.getElementById("title");
  var swap = document.getElementById("swap");

  /* Return path: only a console page (the part after /v2/#), with route characters only. The URL is always built as the
     fixed "/v2/#" plus that route, so a crafted value cannot change the site, the path or the scheme. */
  function nextUrl() {
    var raw = new URLSearchParams(location.search).get("next") || "";
    var m = /^\/v2\/(?:#(\/[A-Za-z0-9\/_\-]*(?:\?[A-Za-z0-9=&_\-.]*)?))?$/.exec(raw);
    return m && m[1] ? "/v2/#" + m[1] : "/v2/";
  }

  function say(kind, text, focus) {
    alertBox.className = "alert " + kind;
    alertBox.setAttribute("role", kind === "bad" ? "alert" : "status");
    alertBox.textContent = text;
    alertBox.hidden = false;
    if (focus) alertBox.focus();
  }
  function quiet() { alertBox.hidden = true; alertBox.textContent = ""; }

  function fieldError(input, text) {
    var err = document.getElementById(input.id + "-err");
    if (err) err.textContent = text || "";
    if (text) input.setAttribute("aria-invalid", "true"); else input.removeAttribute("aria-invalid");
    return !text;
  }
  function clearErrors(form) {
    form.querySelectorAll("input").forEach(function (i) { fieldError(i, ""); });
  }

  function busy(button, on, enLabel, myLabel) {
    button.disabled = on;
    if (on) button.setAttribute("aria-busy", "true"); else button.removeAttribute("aria-busy");
    button.querySelector(".spinner").hidden = !on;
    button.querySelector(".label").textContent = mmT(enLabel, myLabel);
  }

  function setMode(register, keepAlert) {
    loginForm.hidden = register; registerForm.hidden = !register;
    if (!keepAlert) quiet();
    swap.querySelector(".to-register").hidden = register; swap.querySelector(".to-login").hidden = !register;
    title.innerHTML = register ? '<span lang="en">Create account</span><span lang="my">အကောင့် ဖွင့်မည်</span>' : '<span lang="en">Sign in</span><span lang="my">ဝင်မည်</span>';
    var q = new URLSearchParams(location.search);
    if (register) q.set("register", "1"); else q.delete("register");
    try { history.replaceState(null, "", location.pathname + (q.toString() ? "?" + q : "")); } catch (e) {}
    (register ? document.getElementById("r-company") : document.getElementById("email")).focus();
  }
  swap.addEventListener("click", function () { setMode(registerForm.hidden); });
  if (/[?&]register=1/.test(location.search)) setMode(true);

  /* Show or hide a password without losing the cursor position. */
  document.querySelectorAll(".pw-toggle").forEach(function (b) {
    b.addEventListener("click", function () {
      var input = document.getElementById(b.dataset.pw), show = input.type === "password";
      input.type = show ? "text" : "password";
      b.setAttribute("aria-pressed", show ? "true" : "false");
      b.querySelector(".sr-only").innerHTML = show ? '<span lang="en">Hide password</span><span lang="my">စကားဝှက် ဖျောက်ရန်</span>' : '<span lang="en">Show password</span><span lang="my">စကားဝှက် ပြရန်</span>';
      input.focus();
    });
  });
  /* Caps Lock hint while typing a password. */
  document.querySelectorAll('input[name="password"]').forEach(function (input) {
    var hint = document.getElementById(input.id + "-caps");
    function check(e) { if (hint && e.getModifierState) hint.hidden = !e.getModifierState("CapsLock"); }
    input.addEventListener("keydown", check); input.addEventListener("keyup", check);
    input.addEventListener("blur", function () { if (hint) hint.hidden = true; });
  });
  /* Clear a field's error as soon as it is edited. */
  document.querySelectorAll(".auth-card input").forEach(function (i) { i.addEventListener("input", function () { fieldError(i, ""); }); });

  var looksLikeLogin = function (v) { return /^[^\s@]+@[^\s@]+$/.test(v); };
  var looksLikeEmail = function (v) { return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v); };

  function done() {
    say("ok", mmT("Signed in. Opening your console…", "ဝင်ပြီးပါပြီ။ console ကို ဖွင့်နေသည်…"));
    location.href = nextUrl();
  }

  loginForm.addEventListener("submit", async function (e) {
    e.preventDefault();
    var submit = document.getElementById("submit");
    if (submit.disabled) return;
    var email = document.getElementById("email"), password = document.getElementById("password");
    quiet(); clearErrors(loginForm);
    var ok = fieldError(email, !email.value.trim() ? mmT("Enter your email.", "အီးမေးလ် ထည့်ပါ။") : !looksLikeLogin(email.value.trim()) ? mmT("Enter the email you signed up with.", "စာရင်းသွင်းခဲ့သော အီးမေးလ်ကို ထည့်ပါ။") : "");
    ok = fieldError(password, !password.value ? mmT("Enter your password.", "စကားဝှက် ထည့်ပါ။") : "") && ok;
    if (!ok) { (email.getAttribute("aria-invalid") ? email : password).focus(); return; }
    busy(submit, true, "Signing in…", "ဝင်နေသည်…");
    try {
      await MM.post("/auth/login", { email: email.value.trim(), password: password.value });
      done();
    } catch (err) {
      var msg;
      if (err.message === "network-unreachable") msg = mmT("Could not reach the server. Check your connection and try again.", "server သို့ မချိတ်ဆက်နိုင်ပါ။ အင်တာနက်ကို စစ်ပြီး ပြန်ကြိုးစားပါ။");
      else if (err.status === 401 || err.message === "unauthorized") msg = mmT("The email or password is not correct.", "အီးမေးလ် သို့မဟုတ် စကားဝှက် မမှန်ပါ။");
      else if (err.status === 429) msg = mmT("Too many attempts. Wait a few minutes and try again.", "ကြိုးစားမှု များလွန်းသည်။ မိနစ်အနည်းငယ် စောင့်ပြီး ပြန်ကြိုးစားပါ။");
      else if (err.status === 403) msg = mmT("This business account is suspended. Contact us on Telegram.", "ဤလုပ်ငန်းအကောင့်ကို ဆိုင်းငံ့ထားသည်။ Telegram တွင် ဆက်သွယ်ပါ။");
      else msg = mmT("Sign-in did not complete. Try again in a moment.", "ဝင်ခြင်း မပြီးဆုံးပါ။ ခဏနေ ပြန်ကြိုးစားပါ။");
      say("bad", msg, true);
      if (err.status === 401 || err.message === "unauthorized") password.select();
      busy(submit, false, "Sign in", "ဝင်မည်");
    }
  });

  registerForm.addEventListener("submit", async function (e) {
    e.preventDefault();
    var submit = document.getElementById("r-submit");
    if (submit.disabled) return;
    var company = document.getElementById("r-company"), name = document.getElementById("r-name"), email = document.getElementById("r-email"), password = document.getElementById("r-password");
    quiet(); clearErrors(registerForm);
    var ok = fieldError(company, company.value.trim() ? "" : mmT("Enter your business name.", "လုပ်ငန်းအမည် ထည့်ပါ။"));
    ok = fieldError(name, name.value.trim() ? "" : mmT("Enter your name.", "သင့်အမည် ထည့်ပါ။")) && ok;
    ok = fieldError(email, looksLikeEmail(email.value.trim()) ? "" : mmT("Enter a valid email, like name@example.com.", "မှန်ကန်သော အီးမေးလ် ထည့်ပါ (ဥပမာ name@example.com)။")) && ok;
    ok = fieldError(password, password.value.length >= 8 ? "" : mmT("Use 8 or more characters.", "စာလုံး ၈ လုံးနှင့်အထက် သုံးပါ။")) && ok;
    if (!ok) { registerForm.querySelector('[aria-invalid="true"]').focus(); return; }
    busy(submit, true, "Creating your account…", "အကောင့် ဖွင့်နေသည်…");
    try {
      await MM.post("/auth/register", { company_name: company.value.trim(), full_name: name.value.trim(), email: email.value.trim(), password: password.value });
      say("ok", mmT("Account created. Opening your console…", "အကောင့် ဖွင့်ပြီးပါပြီ။ console ကို ဖွင့်နေသည်…"));
      location.href = nextUrl();
    } catch (err) {
      var msg;
      if (err.message === "network-unreachable") msg = mmT("Could not reach the server. Check your connection and try again.", "server သို့ မချိတ်ဆက်နိုင်ပါ။ အင်တာနက်ကို စစ်ပြီး ပြန်ကြိုးစားပါ။");
      else if (err.status === 409) { msg = mmT("This email is already registered. Sign in instead.", "ဤအီးမေးလ်ကို စာရင်းသွင်းပြီးသား ဖြစ်သည်။ ဝင်ရန် ရွှေ့ပါ။"); fieldError(email, msg); }
      else if (err.status === 403) msg = mmT("New accounts are closed right now. Contact us on Telegram.", "ယခု အကောင့်အသစ် ဖွင့်ခြင်း ပိတ်ထားသည်။ Telegram တွင် ဆက်သွယ်ပါ။");
      else if (err.status === 429) msg = mmT("Too many attempts. Wait a minute and try again.", "ကြိုးစားမှု များလွန်းသည်။ ခဏစောင့်ပြီး ပြန်ကြိုးစားပါ။");
      else if (err.status === 400) msg = mmT("Check your details: a valid email and a password of 8 or more characters.", "အချက်အလက်များကို စစ်ပါ - မှန်ကန်သော အီးမေးလ်နှင့် စာလုံး ၈ လုံးနှင့်အထက် စကားဝှက်။");
      else msg = mmT("The account could not be created. Try again in a moment.", "အကောင့် မဖန်တီးနိုင်ပါ။ ခဏနေ ပြန်ကြိုးစားပါ။");
      say("bad", msg, true);
      busy(submit, false, "Create account", "အကောင့် ဖွင့်မည်");
    }
  });

})();
