// Production artifact preview only. Historical Sites/Vite config stays separate.
// Preserve Host so the Go API's same-origin cookie checks remain effective.
export default {
  preview: {
    proxy: {
      '/api': {target: 'http://127.0.0.1:8080', changeOrigin: false},
    },
  },
};
