import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router";
import { App } from "./App";
import "./style.css";

const root = document.getElementById("root");

if (!(root instanceof HTMLElement)) {
  throw new Error("Unable to start CertVault: root element was not found");
}

createRoot(root).render(
  <BrowserRouter>
    <App />
  </BrowserRouter>,
);
