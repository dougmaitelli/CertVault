import fs from "node:fs";
import path from "node:path";

const root = path.resolve(import.meta.dirname, "..");
const outputDir = path.join(root, "public/client");
fs.mkdirSync(outputDir, { recursive: true });
let installer = fs.readFileSync(
  path.join(root, "client/install.template.sh"),
  "utf8",
);
for (const [marker, file] of [
  ["SYNC", "sync.sh"],
  ["LAUNCHER", "launcher.sh"],
  ["UPDATER", "update.sh"],
]) {
  const source = fs
    .readFileSync(path.join(root, "client", file), "utf8")
    .trimEnd();
  installer = installer.replace(`@@${marker}@@`, () => source);
  // The server also needs these assets to assemble signed client update bundles.
  fs.copyFileSync(path.join(root, "client", file), path.join(outputDir, file));
}
const output = path.join(outputDir, "install.sh");
fs.writeFileSync(output, installer);
