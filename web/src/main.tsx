import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router";

import App from "./App.tsx";
import { AuthProvider } from "./auth-context";
import { TooltipProvider } from "./components/ui/tooltip";
import "./i18n";
import "./index.css";
import { initializeTheme } from "./theme";

initializeTheme();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <AuthProvider>
        <TooltipProvider>
          <App />
        </TooltipProvider>
      </AuthProvider>
    </BrowserRouter>
  </StrictMode>,
);
