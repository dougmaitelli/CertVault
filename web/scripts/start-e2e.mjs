import { spawnSync, spawn } from "node:child_process";
import path from "node:path";

const root = path.resolve(import.meta.dirname, "../..");
const env = Object.fromEntries(
  Object.entries(process.env).filter(([key]) => !key.startsWith("CERTVAULT_")),
);
for (const [command, args, cwd] of [
  ["pnpm", ["run", "build"], path.join(root, "web")],
  [
    "go",
    ["build", "-tags=e2e", "-o", "../.cache/e2e/server", "./e2e"],
    path.join(root, "backend"),
  ],
]) {
  const result = spawnSync(command, args, { cwd, env, stdio: "inherit" });
  if (result.error) throw result.error;
  if (result.status !== 0) process.exit(result.status ?? 1);
}
const server = spawn(path.join(root, ".cache/e2e/server"), [], {
  cwd: root,
  env: { ...env, CERTVAULT_UI_DIR: path.join(root, "web/dist") },
  stdio: "inherit",
});
for (const signal of ["SIGTERM", "SIGINT"])
  process.on(signal, () => server.kill(signal));
server.on("error", (error) => {
  throw error;
});
server.on("exit", (code) => process.exit(code ?? 1));
