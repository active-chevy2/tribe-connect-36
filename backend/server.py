"""
Preview-only reverse proxy.

The real application is a Go/Golang binary (see /app/backend/*.go) that serves both
the API (/api/...) and the static Material Design 3 frontend (/app/web). In this
Kubernetes preview environment the ingress routes `/api` traffic to this uvicorn
process on port 8001, so we simply forward every request to the Go app, which runs
on 127.0.0.1:3000 (started by the `frontend` supervisor program).

In production (Docker Compose / Coolify) this file is NOT used at all — the Go
binary is the single service, behind Coolify's reverse proxy.
"""
import os
import httpx
from fastapi import FastAPI, Request, Response

TARGET = os.environ.get("PROXY_TARGET", "http://127.0.0.1:3000")

app = FastAPI()
client = httpx.AsyncClient(base_url=TARGET, timeout=60.0)

HOP_BY_HOP = {
    "content-encoding", "content-length", "transfer-encoding",
    "connection", "keep-alive", "proxy-authenticate",
    "proxy-authorization", "te", "trailers", "upgrade",
}


@app.api_route("/{path:path}", methods=["GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"])
async def proxy(path: str, request: Request):
    url = "/" + path
    if request.url.query:
        url += "?" + request.url.query
    headers = {k: v for k, v in request.headers.items() if k.lower() != "host"}
    body = await request.body()
    try:
        upstream = await client.request(request.method, url, headers=headers, content=body)
    except httpx.ConnectError:
        return Response(content=b'{"error":"backend starting, retry shortly"}',
                        status_code=503, media_type="application/json")
    out_headers = {k: v for k, v in upstream.headers.items() if k.lower() not in HOP_BY_HOP}
    return Response(content=upstream.content, status_code=upstream.status_code, headers=out_headers)


@app.on_event("shutdown")
async def _shutdown():
    await client.aclose()
