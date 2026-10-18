// server.ts — простой просмотрщик дизайна интерфейса TNS65 (владелец, 18.10).
// Запуск:  bun server.ts          (из папки interface; порт по умолчанию 8788, можно PORT=9000 bun server.ts)
// Раздаёт эту папку как корень: страницы открывают /static/... абсолютными путями — они работают как есть.
// Корень «/» — список всех .html страниц папки. Кэш отключён, чтобы правки дизайна были видны сразу.

import { createServer, type IncomingMessage, type ServerResponse } from "node:http";
import { readFile, readdir } from "node:fs/promises";
import { extname, join, normalize, resolve, sep } from "node:path";
import { networkInterfaces } from "node:os";

const meta = import.meta as { dir?: string; dirname?: string };
const ROOT = meta.dir ?? meta.dirname ?? process.cwd();
const PORT = Number(process.env.PORT || 8788);

const MIME: Record<string, string> = {
  ".html": "text/html; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".txt": "text/plain; charset=utf-8",
  ".md": "text/plain; charset=utf-8",
  ".png": "image/png",
  ".jpg": "image/jpeg",
  ".jpeg": "image/jpeg",
  ".gif": "image/gif",
  ".svg": "image/svg+xml",
  ".ico": "image/x-icon",
  ".woff": "font/woff",
  ".woff2": "font/woff2",
  ".ttf": "font/ttf",
  ".otf": "font/otf",
};

function esc(s: string): string {
  return s.replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c] as string);
}

async function pageIndex(): Promise<string> {
  const files = (await readdir(ROOT)).filter((f) => f.endsWith(".html")).sort();
  const items = files
    .map((f) => `<li><a href="/${encodeURIComponent(f)}">${esc(f)}</a></li>`)
    .join("\n");
  return `<!doctype html><html><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>TNS65 — дизайн интерфейса</title>
<style>body{font:16px/1.6 system-ui,sans-serif;margin:0;padding:24px;background:#14161a;color:#e6e6e6}
h1{font-size:18px}ul{list-style:none;padding:0;max-width:640px}li{border-bottom:1px solid #2a2d33}
a{display:block;padding:12px 8px;color:#7ab0ff;text-decoration:none}a:active{background:#1e2127}
small{color:#8a8f98}</style></head><body>
<h1>TNS65 — дизайн интерфейса (${files.length} страниц)</h1>
<small>Папка interface, раздача без кэша. Сервер: bun server.ts, PORT=${PORT}</small>
<ul>${items}</ul></body></html>`;
}

async function handle(req: IncomingMessage, res: ServerResponse): Promise<void> {
  const url = new URL(req.url ?? "/", "http://local");
  const pathname = decodeURIComponent(url.pathname);

  if (req.method !== "GET" && req.method !== "HEAD") {
    res.writeHead(405, { "Content-Type": "text/plain; charset=utf-8" });
    res.end("405 method not allowed");
    return;
  }

  res.setHeader("Cache-Control", "no-store");

  if (pathname === "/") {
    res.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
    res.end(req.method === "HEAD" ? undefined : await pageIndex());
    return;
  }

  // защита от выхода за пределы папки
  const file = resolve(ROOT, "." + normalize(pathname));
  if (file !== ROOT && !file.startsWith(ROOT + sep)) {
    res.writeHead(403, { "Content-Type": "text/plain; charset=utf-8" });
    res.end("403 outside root");
    return;
  }

  try {
    const body = await readFile(file);
    res.writeHead(200, { "Content-Type": MIME[extname(file).toLowerCase()] ?? "application/octet-stream" });
    res.end(req.method === "HEAD" ? undefined : body);
  } catch {
    res.writeHead(404, { "Content-Type": "text/plain; charset=utf-8" });
    res.end("404 not found: " + pathname);
  }
}

const server = createServer((req, res) => {
  handle(req, res).catch((err) => {
    res.writeHead(500, { "Content-Type": "text/plain; charset=utf-8" });
    res.end("500 " + String(err));
  });
});

server.listen(PORT, "0.0.0.0", () => {
  const ips = Object.values(networkInterfaces())
    .flat()
    .filter((n): n is NonNullable<typeof n> => !!n && n.family === "IPv4" && !n.internal)
    .map((n) => n.address);
  console.log(`[design-viewer] root: ${ROOT}`);
  console.log(`[design-viewer] http://localhost:${PORT}/ — список страниц`);
  for (const ip of ips) console.log(`[design-viewer] http://${ip}:${PORT}/ — с телефона`);
});
