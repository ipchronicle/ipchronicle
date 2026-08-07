import { BrowserRouter, Route, Routes } from "react-router";

import { AppHeader } from "@/components/app-header";
import { TooltipProvider } from "@/components/ui/tooltip";
import { SystemStatusPage } from "@/pages/system-status-page";

function App() {
  return (
    <BrowserRouter>
      <TooltipProvider>
        <div className="min-h-svh bg-background">
          <AppHeader />
          <Routes>
            <Route path="*" element={<SystemStatusPage />} />
          </Routes>
        </div>
      </TooltipProvider>
    </BrowserRouter>
  );
}

export default App;
