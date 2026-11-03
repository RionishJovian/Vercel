(function () {
  var d = document.documentElement;
  var btns = document.querySelectorAll("[data-set-lang]");
  function show(l) {
    d.dataset.lang = l; d.lang = l;
    btns.forEach(function (b) { b.setAttribute("aria-pressed", b.dataset.setLang === l ? "true" : "false"); });
    if (demo) loadDemo();
  }
  /* Preview video: right language, only plays while on screen, still image if motion is reduced. */
  var demo = document.getElementById("demo");
  var still = matchMedia("(prefers-reduced-motion: reduce)").matches;
  var visible = false;
  function loadDemo() {
    var l = d.dataset.lang === "en" ? "en" : "my";
    var src = "/assets/video/portal-demo-" + l + ".mp4";
    if (demo.getAttribute("src") !== src) { demo.setAttribute("src", src); demo.setAttribute("poster", "/assets/video/portal-demo-" + l + ".jpg"); }
    if (visible && !still) { var p = demo.play(); if (p && p.catch) p.catch(function () {}); }
  }
  if ("IntersectionObserver" in window) new IntersectionObserver(function (e) { visible = e[0].isIntersecting; if (visible) loadDemo(); else demo.pause(); }, { threshold: .25 }).observe(demo);
  loadDemo();
  show(d.dataset.lang === "en" ? "en" : "my");
  btns.forEach(function (b) { b.addEventListener("click", function () { show(b.dataset.setLang); try { localStorage.setItem("mm2_lang", b.dataset.setLang); } catch (e) {} }); });
  document.getElementById("theme").addEventListener("click", function () {
    var n = d.dataset.theme === "day" ? "night" : "day";
    d.dataset.theme = n;
    try { localStorage.setItem("mm_theme", n); } catch (e) {}
  });
})();
