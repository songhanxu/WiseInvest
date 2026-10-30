import React from "react";
import ReactDOM from "react-dom/client";
import { App } from "./App";
import "./styles.css";
import "./design-system.css";
import "./research.css";
import "./login.css";
import "./theme.css";

document.documentElement.dataset.theme = localStorage.getItem("wiseinvest.theme") === "dark" ? "dark" : "light";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
