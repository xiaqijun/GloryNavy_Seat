import { spawn } from "node:child_process";
import { existsSync, readFileSync, copyFileSync, mkdirSync } from "node:fs";
import { parseEnv } from "node:util";
import { fileURLToPath } from "node:url";
import path from "node:path";

const root = fileURLToPath(new URL("../", import.meta.url));
process.chdir(root);
const win = process.platform === "win32";
const localGo = path.join(root, ".tools/go/bin");
if (existsSync(localGo))
  process.env.PATH = localGo + path.delimiter + process.env.PATH;
function loadLocalEnv() {
  if (!existsSync(".env")) return;
  for (const [key, value] of Object.entries(
    parseEnv(readFileSync(".env", "utf8")),
  )) {
    if (process.env[key] === undefined) process.env[key] = value;
  }
}
loadLocalEnv();
const children = new Set();
const npmCLI =
  process.env.npm_execpath ||
  path.join(
    path.dirname(process.execPath),
    win
      ? "node_modules/npm/bin/npm-cli.js"
      : "../lib/node_modules/npm/bin/npm-cli.js",
  );
let stopping = false;
function stop() {
  stopping = true;
  for (const child of children) {
    if (win)
      spawn("taskkill", ["/pid", String(child.pid), "/t", "/f"], {
        windowsHide: true,
        stdio: "ignore",
      });
    else {
      try {
        process.kill(-child.pid, "SIGTERM");
      } catch {
        /* Already exited. */
      }
    }
  }
}
process.on("SIGINT", stop);
process.on("SIGTERM", stop);
function run(command, args = [], cwd = root) {
  return new Promise((resolve, reject) => {
    if (stopping) {
      reject(new Error("Task interrupted"));
      return;
    }
    const useCLI = command === "npm" && existsSync(npmCLI);
    if (command === "npm" && win && !useCLI) {
      reject(new Error("npm CLI not found; invoke this task through npm run"));
      return;
    }
    const child = spawn(
      useCLI ? process.execPath : command,
      useCLI ? [npmCLI, ...args] : args,
      { cwd, stdio: "inherit", windowsHide: true, detached: !win },
    );
    children.add(child);
    child.once("error", (error) => {
      children.delete(child);
      reject(error);
    });
    child.once("exit", (code) => {
      children.delete(child);
      code === 0 || stopping
        ? resolve()
        : reject(new Error(`${command} exited with ${code}`));
    });
  });
}
const web = (...args) => run("npm", args, path.join(root, "web"));
const migrate = async () => {
  await run("go", ["run", "./cmd/migrate", "up"]);
  await run("go", ["run", "./cmd/jobs-migrate"]);
};
const binary = path.join(root, "bin", win ? "server.exe" : "server");
async function buildAPI() {
  mkdirSync("bin", { recursive: true });
  await run("go", ["build", "-trimpath", "-o", binary, "./cmd/server"]);
}
async function main() {
  switch (process.argv[2]) {
    case "sde-import":
      await run("go", ["run", "./cmd/sde-import", ...process.argv.slice(3)]);
      break;
    case "access-admin":
      await run("go", ["run", "./cmd/access-admin", ...process.argv.slice(3)]);
      break;
    case "setup":
      if (!existsSync(".env")) copyFileSync(".env.example", ".env");
      await run("go", ["mod", "download"]);
      await web("ci");
      break;
    case "dev":
    case "dev-external":
      if (!existsSync(".env")) copyFileSync(".env.example", ".env");
      loadLocalEnv();
      if (!existsSync("web/node_modules")) await web("ci");
      if (process.argv[2] === "dev")
        await run("docker", [
          "compose",
          "-f",
          "compose.dev.yaml",
          "up",
          "-d",
          "--wait",
        ]);
      await migrate();
      await buildAPI();
      if (!process.env.API_PROXY_TARGET)
        process.env.API_PROXY_TARGET = `http://${process.env.HTTP_ADDR || "127.0.0.1:8080"}`;
      await Promise.race([run(binary), web("run", "dev")]);
      stop();
      break;
    case "api":
      await buildAPI();
      await run(binary);
      break;
    case "migrate":
      await migrate();
      break;
    case "generate": {
      const sqlc = path.join(root, ".tools/sqlc", win ? "sqlc.exe" : "sqlc");
      await run(
        existsSync(sqlc) ? sqlc : "go",
        existsSync(sqlc)
          ? ["generate"]
          : ["run", "github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1", "generate"],
      );
      break;
    }
    case "check":
      await run("go", ["vet", "./..."]);
      await run("go", ["test", "./..."]);
      await web("run", "lint");
      await web("run", "test");
      await web("run", "build");
      break;
    case "build":
      await buildAPI();
      await web("run", "build");
      break;
    default:
      throw new Error("Unknown task");
  }
}
main().catch((error) => {
  console.error(error.message);
  stop();
  process.exitCode = 1;
});
