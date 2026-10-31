import "@fontsource-variable/bricolage-grotesque";
import "@fontsource-variable/instrument-sans";
import "@fontsource/noto-sans-myanmar/400.css";
import "@fontsource/noto-sans-myanmar/600.css";
import "./styles/tokens.css";
import "./styles/base.css";
import "./styles/shell.css";
import "./styles/pages.css";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { SWRConfig } from "swr";
import { App } from "./App";
import { I18nProvider } from "./i18n";
import { ThemeProvider } from "./lib/theme";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ThemeProvider><I18nProvider>
      <SWRConfig value={{ shouldRetryOnError: false, dedupingInterval: 4000 }}><App /></SWRConfig>
    </I18nProvider></ThemeProvider>
  </StrictMode>,
);
