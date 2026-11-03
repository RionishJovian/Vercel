/* Same keys as the app so Day/Night and language carry over after sign-in. */
(function () {
  var d = document.documentElement;
  try {
    var t = localStorage.getItem("mm_theme");
    d.dataset.theme = t === "day" || t === "night" ? t : (matchMedia("(prefers-color-scheme: light)").matches ? "day" : "night");
    var l = localStorage.getItem("mm2_lang");
    if (l === "en" || l === "my") { d.dataset.lang = l; d.lang = l; }
  } catch (e) {}
})();
