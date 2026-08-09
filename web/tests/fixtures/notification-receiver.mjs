import http from "node:http";

const port = Number.parseInt(process.env.PORT ?? "19090", 10);
const maximumBodyBytes = 2 * 1024 * 1024;
let messages = [];

function json(response, status, value) {
  response.writeHead(status, { "Content-Type": "application/json" });
  response.end(JSON.stringify(value));
}

async function readBody(request) {
  const chunks = [];
  let size = 0;
  for await (const chunk of request) {
    size += chunk.length;
    if (size > maximumBodyBytes) throw new Error("request body too large");
    chunks.push(chunk);
  }
  return Buffer.concat(chunks).toString("utf8");
}

const server = http.createServer(async (request, response) => {
  try {
    if (request.method === "GET" && request.url === "/healthz") {
      response.writeHead(204);
      response.end();
      return;
    }
    if (request.method === "GET" && request.url === "/messages") {
      json(response, 200, { items: messages });
      return;
    }
    if (request.method === "POST" && request.url === "/reset") {
      messages = [];
      response.writeHead(204);
      response.end();
      return;
    }
    if (request.method === "POST" && request.url === "/delivery") {
      messages.push({
        body: await readBody(request),
        headers: request.headers,
      });
      response.writeHead(204);
      response.end();
      return;
    }
    response.writeHead(404);
    response.end();
  } catch {
    response.writeHead(413);
    response.end();
  }
});

server.listen(port, "0.0.0.0");
