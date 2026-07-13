// Keep the stylesheet on the same lazy route as the settings implementation.
// Direct unit tests can import SettingsPanel without teaching Node how to load
// CSS, while Vite still waits for the route stylesheet before rendering it.
import "./SettingsPanel.css";

export { SettingsPanel } from "./SettingsPanel";
