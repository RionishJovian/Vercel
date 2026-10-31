import { api } from "../api.js";
import { h } from "../dom.js";
import { t, getLang, setLang } from "../i18n.js";

export default function signin({ notice, onDone }) {
  const message = h("p", { class: "note bad", role: "alert", hidden: !notice }, notice || "");
  message.hidden = !notice;
  const email = h("input", { class: "input", id: "email", type: "email", name: "email", autocomplete: "username", required: true });
  const password = h("input", { class: "input", id: "password", type: "password", name: "password", autocomplete: "current-password", required: true });
  const submit = h("button", { class: "btn", type: "submit" }, t("s.submit"));

  const form = h("form", { class: "glass", novalidate: true, onsubmit: async (e) => {
    e.preventDefault();
    if (!form.reportValidity()) return;
    submit.disabled = true;
    submit.textContent = t("s.working");
    message.hidden = true;
    try {
      await api.post("/auth/login", { email: email.value.trim(), password: password.value });
      onDone();
    } catch (err) {
      message.textContent = err.message === "network-unreachable" ? t("s.network") : t("s.failed");
      message.hidden = false;
      submit.disabled = false;
      submit.textContent = t("s.submit");
    }
  } },
    h("label", { class: "field", for: "email" }, h("span", null, t("s.email")), email),
    h("label", { class: "field", for: "password" }, h("span", null, t("s.password")), password),
    message,
    submit);

  const other = getLang() === "my" ? "en" : "my";
  return h("div", { class: "signin" },
    h("div", { class: "stack" },
      h("span", { class: "eyebrow" }, "Mega Magic"),
      h("h1", { class: "big" }, t("title.signin"))),
    form,
    h("button", { class: "pill", type: "button", onclick: () => { setLang(other); location.reload(); } }, other === "en" ? "English" : "မြန်မာ"));
}
