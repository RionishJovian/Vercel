/* Compatibility for the pre-migration console, only in the optional rollback web image.
   Serve /app/... in place; redirecting back to /v2/ could loop through a browser's cached 301. */
(function () {
  if (!location.pathname.startsWith('/app/')) return;
  if (/^#\/|^#portal\b/.test(location.hash)) return;
  var page = location.pathname.slice('/app/'.length) || 'home';
  history.replaceState(history.state, '', location.pathname + location.search + '#/' + page + location.search);
})();
