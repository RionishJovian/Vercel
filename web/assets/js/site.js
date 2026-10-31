/* Shared by the pricing and sign-in pages: Day/Night and English/Burmese, same keys as the app. */
(function () {
  var d = document.documentElement;
  var btns = document.querySelectorAll("[data-set-lang]");
  function show(l) {
    d.dataset.lang = l; d.lang = l;
    btns.forEach(function (b) { b.setAttribute("aria-pressed", b.dataset.setLang === l ? "true" : "false"); });
    document.dispatchEvent(new CustomEvent("mm-lang", { detail: l }));
  }
  window.mmLang = function () { return d.dataset.lang === "en" ? "en" : "my"; };
  window.mmT = function (en, my) { return window.mmLang() === "en" ? en : my; };
  show(window.mmLang());
  btns.forEach(function (b) { b.addEventListener("click", function () { show(b.dataset.setLang); try { localStorage.setItem("mm2_lang", b.dataset.setLang); } catch (e) {} }); });
  var theme = document.getElementById("theme");
  if (theme) theme.addEventListener("click", function () {
    var n = d.dataset.theme === "day" ? "night" : "day";
    d.dataset.theme = n;
    try { localStorage.setItem("mm_theme", n); } catch (e) {}
  });
})();
