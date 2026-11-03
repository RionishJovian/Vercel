var alertBox = document.getElementById("alert");
var loginForm = document.getElementById("login-form");
var registerForm = document.getElementById("register-form");
var title = document.getElementById("title");
var swap = document.getElementById("swap");
function setMode(register) {
  loginForm.hidden = register; registerForm.hidden = !register; alertBox.hidden = true;
  swap.querySelector(".to-register").hidden = register; swap.querySelector(".to-login").hidden = !register;
  title.innerHTML = register ? '<span lang="en">Create account</span><span lang="my">အကောင့် ဖွင့်မည်</span>' : '<span lang="en">Sign in</span><span lang="my">ဝင်မည်</span>';
  try { history.replaceState(null, "", register ? "?register=1" : location.pathname); } catch (e) {}
  (register ? document.getElementById("r-company") : document.getElementById("email")).focus();
}
swap.addEventListener("click", function () { setMode(registerForm.hidden); });
if (/[?&]register=1/.test(location.search)) setMode(true);
function fail(text) { alertBox.hidden = false; alertBox.textContent = text; }

loginForm.addEventListener("submit", async function (e) {
  e.preventDefault();
  var submit = document.getElementById("submit");
  alertBox.hidden = true; submit.disabled = true;
  submit.textContent = mmT("Signing in…", "ဝင်နေသည်…");
  try {
    await MM.post("/auth/login", { email: document.getElementById("email").value.trim(), password: document.getElementById("password").value });
    location.href = "/v2/";
  } catch (err) {
    fail(err.message === "network-unreachable"
      ? mmT("Could not reach the server. Try again.", "server သို့ မချိတ်ဆက်နိုင်ပါ။ ပြန်ကြိုးစားပါ။")
      : mmT("Sign-in failed: ", "ဝင်ခြင်း မအောင်မြင်ပါ - ") + err.message);
    submit.disabled = false;
    submit.textContent = mmT("Sign in", "ဝင်မည်");
  }
});

registerForm.addEventListener("submit", async function (e) {
  e.preventDefault();
  var submit = document.getElementById("r-submit");
  var password = document.getElementById("r-password").value;
  alertBox.hidden = true;
  if (password.length < 8) { fail(mmT("The password needs 8 or more characters.", "စကားဝှက်သည် စာလုံး ၈ လုံးနှင့်အထက် ရှိရမည်။")); return; }
  submit.disabled = true;
  submit.textContent = mmT("Creating…", "ဖန်တီးနေသည်…");
  try {
    await MM.post("/auth/register", {
      company_name: document.getElementById("r-company").value.trim(),
      full_name: document.getElementById("r-name").value.trim(),
      email: document.getElementById("r-email").value.trim(),
      password: password
    });
    location.href = "/v2/";
  } catch (err) {
    var msg;
    if (err.message === "network-unreachable") msg = mmT("Could not reach the server. Try again.", "server သို့ မချိတ်ဆက်နိုင်ပါ။ ပြန်ကြိုးစားပါ။");
    else if (err.status === 409) msg = mmT("This email is already registered. Sign in instead.", "ဤအီးမေးလ်ကို စာရင်းသွင်းပြီးသား ဖြစ်သည်။ ဝင်ရန် ရွှေ့ပါ။");
    else if (err.status === 403) msg = mmT("Registration is closed right now. Contact us on Telegram.", "ယခု စာရင်းသွင်းခြင်း ပိတ်ထားသည်။ Telegram တွင် ဆက်သွယ်ပါ။");
    else if (err.status === 429) msg = mmT("Too many attempts. Wait a minute and try again.", "ကြိုးစားမှု များလွန်းသည်။ ခဏစောင့်ပြီး ပြန်ကြိုးစားပါ။");
    else if (err.status === 400) msg = mmT("Check your details. Use a valid email and a password of 8 or more characters.", "အချက်အလက်များကို စစ်ပါ။ မှန်ကန်သော အီးမေးလ်နှင့် စာလုံး ၈ လုံးနှင့်အထက် စကားဝှက် ထည့်ပါ။");
    else msg = mmT("Could not create the account. Try again.", "အကောင့် မဖန်တီးနိုင်ပါ။ ပြန်ကြိုးစားပါ။");
    fail(msg);
    submit.disabled = false;
    submit.textContent = mmT("Create account", "အကောင့် ဖွင့်မည်");
  }
});
