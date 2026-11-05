// Stands in for satellite.js's WebAssembly builds, which this app never calls. See vite.config.ts.
export default function noWasm(): never { throw new Error("satellite.js WebAssembly is not bundled"); }
