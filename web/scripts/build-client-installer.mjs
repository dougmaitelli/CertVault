import fs from "node:fs";
import path from "node:path";

const root = path.resolve(import.meta.dirname, "..");
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
    .readFileSync(path.join(root, "public/client", file), "utf8")
    .trimEnd();
  installer = installer.replace(`@@${marker}@@`, () => source);
}
const output = path.join(root, "public/client/install.sh");
if (process.argv.includes("--check")) {
  if (fs.readFileSync(output, "utf8") !== installer)
    throw new Error(
      "Generated client installer is stale; run node scripts/build-client-installer.mjs",
    );
} else fs.writeFileSync(output, installer);
