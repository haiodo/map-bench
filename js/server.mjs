// Стенд для браузеров: node js/server.mjs, затем open -a Safari "http://localhost:8765/browser.html".
// COOP/COEP включают cross-origin isolation, иначе браузер огрубляет performance.now до 0.1-1 мс.
import { createServer } from "node:http";
import { appendFileSync, readFileSync, existsSync } from "node:fs";
import { join, extname } from "node:path";

const port = Number(process.env.PORT ?? 8765);
const root = new URL(".", import.meta.url).pathname;
const out = process.env.OUT ?? join(root, "../results/raw/D-browser.jsonl");
const types = { ".html": "text/html", ".mjs": "text/javascript", ".js": "text/javascript" };

createServer((req, res) => {
  const headers = { "Cross-Origin-Opener-Policy": "same-origin", "Cross-Origin-Embedder-Policy": "require-corp" };
  if (req.method === "POST" && req.url === "/result") {
    let body = "";
    req.on("data", (c) => (body += c));
    req.on("end", () => {
      const line = JSON.parse(body);
      appendFileSync(out, JSON.stringify(line) + "\n");
      console.error(`${line.runtime.slice(0, 40)} ${line.impl} ${line.keys} n=${line.n} ${line.metric} ${line.median.toFixed(2)}`);
      res.writeHead(204, headers).end();
    });
    return;
  }
  if (req.url === "/done") {
    console.error("браузер закончил");
    res.writeHead(204, headers).end();
    // run.sh ждёт выхода сервера как сигнала, что браузер прошёл все конфигурации
    if (process.env.EXIT_ON_DONE) setTimeout(() => process.exit(0), 500);
    return;
  }
  const file = join(root, new URL(req.url, "http://x").pathname);
  if (!file.startsWith(root) || !existsSync(file)) {
    res.writeHead(404, headers).end();
    return;
  }
  res.writeHead(200, { ...headers, "Content-Type": types[extname(file)] ?? "application/octet-stream" }).end(readFileSync(file));
}).listen(port, "127.0.0.1", () => console.error(`http://localhost:${port}/browser.html -> ${out}`));
