import { Route, Routes } from "react-router-dom";
import { ComingSoonPage } from "../features/coming-soon/ComingSoonPage";

/**
 * The portal's routes. Until the portal exists every address shows the holding page; when it does, the
 * login and the customer's screens are added here (see the README for the plan).
 */
export default function App() {
  return (
    <Routes>
      <Route path="*" element={<ComingSoonPage />} />
    </Routes>
  );
}
