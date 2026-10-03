/* MikroTik Magic shared appearance + icon system.
 * Runs before paint so the persisted Day/Night choice never flashes.
 * Icons are local, dependency-free Lucide paths (24px viewBox, currentColor).
 */
(function () {
  "use strict";

  var storageKey = "mm_theme";
  var allowed = { day: true, night: true };
  var paths = {
    menu: '<path d="M4 12h16M4 6h16M4 18h16"/>',
    sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41"/>',
    moon: '<path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79Z"/>',
    dashboard: '<rect width="7" height="9" x="3" y="3" rx="1"/><rect width="7" height="5" x="14" y="3" rx="1"/><rect width="7" height="9" x="14" y="12" rx="1"/><rect width="7" height="5" x="3" y="16" rx="1"/>',
    activity: '<path d="M22 12h-4l-3 9L9 3l-3 9H2"/>',
    network: '<rect x="16" y="16" width="6" height="6" rx="1"/><rect x="2" y="16" width="6" height="6" rx="1"/><rect x="9" y="2" width="6" height="6" rx="1"/><path d="M5 16v-3h14v3M12 13V8"/>',
    alert: '<path d="m21.73 18-8-14a2 2 0 0 0-3.46 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"/><path d="M12 9v4M12 17h.01"/>',
    router: '<rect width="20" height="8" x="2" y="14" rx="2"/><path d="M6.01 18H6M10 18h.01M15 10v4M12 2a7 7 0 0 1 7 7M12 6a3 3 0 0 1 3 3"/>',
    bot: '<rect width="18" height="10" x="3" y="11" rx="2"/><circle cx="12" cy="5" r="2"/><path d="M12 7v4M8 16h.01M16 16h.01"/>',
    shield: '<path d="M20 13c0 5-3.5 7.5-8 9-4.5-1.5-8-4-8-9V5l8-3 8 3v8Z"/><path d="m9 12 2 2 4-4"/>',
    backup: '<ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M3 5v7c0 1.7 4 3 9 3s9-1.3 9-3V5M3 12v7c0 1.7 4 3 9 3 1.2 0 2.3-.1 3.3-.2"/><path d="m19 16-2 2 2 2M17 18h4"/>',
    scan: '<path d="M3 7V5a2 2 0 0 1 2-2h2M17 3h2a2 2 0 0 1 2 2v2M21 17v2a2 2 0 0 1-2 2h-2M7 21H5a2 2 0 0 1-2-2v-2"/><circle cx="12" cy="12" r="3"/>',
    template: '<rect width="18" height="18" x="3" y="3" rx="2"/><path d="M3 9h18M9 21V9"/>',
    terminal: '<polyline points="4 17 10 11 4 5"/><line x1="12" x2="20" y1="19" y2="19"/>',
    wifi: '<path d="M12 20h.01M2 8.82a15 15 0 0 1 20 0M5 12.86a10 10 0 0 1 14 0M8.5 16.43a5 5 0 0 1 7 0"/>',
    ticket: '<path d="M2 9a3 3 0 0 0 0 6v4a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-4a3 3 0 0 0 0-6V5a2 2 0 0 0-2-2H4a2 2 0 0 0-2 2Z"/><path d="M13 5v2M13 17v2M13 11v2"/>',
    coins: '<circle cx="8" cy="8" r="6"/><path d="M18.09 10.37A6 6 0 1 1 10.34 18M7 6h1v4M16.71 13.88l.7.71-2.82 2.82"/>',
    key: '<circle cx="7.5" cy="15.5" r="5.5"/><path d="m21 2-9.6 9.6M15 6l3 3M18 3l3 3"/>',
    users: '<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75"/>',
    log: '<path d="M6 2h9l5 5v15H6z"/><path d="M14 2v6h6M9 13h6M9 17h6"/>',
    gauge: '<path d="m12 14 4-4M3.34 19a10 10 0 1 1 17.32 0"/><path d="M9 19h6"/>',
    languages: '<path d="m5 8 6 6M4 14l6-6 2-3M2 5h12M7 2h1M22 22l-5-10-5 10M14 18h6"/>',
    book: '<path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20V3H6.5A2.5 2.5 0 0 0 4 5.5z"/><path d="M4 5.5v14M8 7h8M8 11h6"/>',
    user: '<circle cx="12" cy="8" r="4"/><path d="M4 22a8 8 0 0 1 16 0"/>',
    chart: '<path d="M3 3v18h18"/><path d="m7 16 4-5 4 3 5-7"/>',
    building: '<rect width="16" height="20" x="4" y="2" rx="2"/><path d="M9 22v-4h6v4M8 6h.01M16 6h.01M8 10h.01M16 10h.01M8 14h.01M16 14h.01"/>',
    logout: '<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9"/>'
  };
  var viewIcons = {
    gateways: "router", network: "network", wan: "activity", pppoe: "users", wifi: "wifi", existing: "template", media: "template",
    overview: "dashboard", fleet: "activity", topology: "network", incidents: "alert", sites: "building", routers: "router",
    security: "shield", backups: "backup", scans: "scan", templates: "template",
    termlog: "terminal", portal: "wifi", vouchers: "ticket", store: "coins", coins: "coins", unlockkeys: "key",
    team: "users", events: "log", aiusage: "gauge", i18n: "languages", revenue: "chart",
    guide: "book", profile: "user", platform: "building"
  };

  function icon(name, size) {
    var body = paths[name] || paths.dashboard;
    var px = Number(size) || 20;
    return '<svg class="mm-icon" aria-hidden="true" width="' + px + '" height="' + px + '" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">' + body + '</svg>';
  }

  function preferredTheme() {
    try {
      var saved = localStorage.getItem(storageKey);
      if (allowed[saved]) return saved;
    } catch (_) {}
    return window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches ? "night" : "day";
  }

  function setTheme(theme, persist) {
    var next = allowed[theme] ? theme : "day";
    document.documentElement.dataset.theme = next;
    document.documentElement.style.colorScheme = next === "night" ? "dark" : "light";
    if (persist !== false) {
      try { localStorage.setItem(storageKey, next); } catch (_) {}
    }
    document.querySelectorAll("[data-theme-toggle]").forEach(function (button) {
      var night = next === "night";
      button.innerHTML = icon(night ? "sun" : "moon", 18) + '<span class="theme-label">' + (night ? "Day mode" : "Night mode") + "</span>";
      button.setAttribute("aria-label", night ? "Switch to Day mode" : "Switch to Night mode");
      button.setAttribute("title", night ? "Switch to Day mode" : "Switch to Night mode");
      button.setAttribute("aria-pressed", night ? "true" : "false");
    });
    window.dispatchEvent(new CustomEvent("mm:themechange", { detail: { theme: next } }));
  }

  function decorate() {
    if (!document.querySelector("[data-theme-toggle]") && document.body) {
      var publicToggle = document.createElement("button");
      publicToggle.type = "button";
      publicToggle.className = "theme-toggle public-theme-toggle";
      publicToggle.setAttribute("data-theme-toggle", "");
      document.body.appendChild(publicToggle);
    }
    document.querySelectorAll(".nav-item[data-view]").forEach(function (item) {
      if (item.querySelector(".mm-icon")) return;
      item.insertAdjacentHTML("afterbegin", icon(viewIcons[item.dataset.view] || "dashboard", 18));
    });
    document.querySelectorAll("[data-lucide]").forEach(function (item) {
      if (!item.querySelector(".mm-icon")) item.innerHTML = icon(item.dataset.lucide, item.dataset.iconSize || 24);
    });
    var signout = document.getElementById("signout");
    if (signout && !signout.querySelector(".mm-icon")) signout.insertAdjacentHTML("afterbegin", icon("logout", 18));
    var menu = document.getElementById("rail-toggle");
    if (menu) {
      var visual = menu.querySelector("[aria-hidden=true]");
      if (visual) visual.innerHTML = icon("menu", 19);
    }
    document.querySelectorAll("[data-theme-toggle]").forEach(function (button) {
      if (!button.dataset.themeBound) {
        button.dataset.themeBound = "true";
        button.addEventListener("click", function () {
          setTheme(document.documentElement.dataset.theme === "night" ? "day" : "night", true);
        });
      }
    });
    setTheme(document.documentElement.dataset.theme || preferredTheme(), false);
  }

  document.documentElement.dataset.theme = preferredTheme();
  window.MMTheme = { get: function () { return document.documentElement.dataset.theme; }, set: setTheme, icon: icon };
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", decorate);
  else decorate();
})();
