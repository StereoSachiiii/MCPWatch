import re
import httpx
from fastapi import FastAPI
from starlette.routing import Route, Mount
from starlette.applications import Starlette
import uvicorn
from mcp.server import Server
from mcp.server.sse import SseServerTransport
from mcp.server.models import InitializationOptions
import mcp.types as types

# 1. Initialize FastAPI and MCP Server
app = FastAPI(title="FastAPI Web Access MCP Server")
mcp_server = Server("web-access-server")

# 2. Define the Web Page Scraper Tool
@mcp_server.list_tools()
async def handle_list_tools() -> list[types.Tool]:
    return [
        types.Tool(
            name="fetch_webpage",
            description="Fetches the content of a web page, strips HTML tags, and returns its plain text representation.",
            inputSchema={
                "type": "object",
                "properties": {
                    "url": {
                        "type": "string",
                        "description": "The absolute HTTP/HTTPS URL of the webpage to fetch"
                    }
                },
                "required": ["url"]
            }
        )
    ]

# 3. Handle Tool Calls
@mcp_server.call_tool()
async def handle_call_tool(name: str, arguments: dict) -> list[types.TextContent]:
    if name != "fetch_webpage":
        return [types.TextContent(type="text", text=f"Error: Unknown tool '{name}'")]

    url = arguments.get("url")
    if not url:
        return [types.TextContent(type="text", text="Error: Missing URL argument.")]

    print(f"🕸️ Tool Fetching URL: {url}")
    try:
        headers = {
            "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
        }
        async with httpx.AsyncClient(timeout=15.0, follow_redirects=True) as client:
            resp = await client.get(url, headers=headers)
            resp.raise_for_status()
            
            # Basic HTML stripping logic
            html = resp.text
            # Remove scripts and styles
            html = re.sub(r'<(script|style).*?>.*?</\1>', '', html, flags=re.DOTALL | re.IGNORECASE)
            # Remove remaining HTML tags
            text = re.sub(r'<[^>]*>', ' ', html)
            # Compress spaces
            text = re.sub(r'\s+', ' ', text).strip()
            
            # Truncate content to 6000 chars to avoid token limit issues
            if len(text) > 6000:
                text = text[:6000] + "... [truncated]"
                
            return [types.TextContent(type="text", text=text)]
            
    except Exception as e:
        print(f"❌ Error fetching {url}: {e}")
        return [types.TextContent(type="text", text=f"Error fetching webpage: {str(e)}")]

# 4. Integrate MCP server with SseServerTransport
sse_transport = SseServerTransport("/mcp/messages/")

async def handle_sse(request):
    async with sse_transport.connect_sse(
        request.scope, request.receive, request._send
    ) as (in_stream, out_stream):
        await mcp_server.run(
            in_stream,
            out_stream,
            InitializationOptions(
                server_name="web-access-server",
                server_version="1.0.0",
                capabilities=types.ServerCapabilities(tools=types.ToolsCapability())
            )
        )

# Mount the MCP routes directly on FastAPI
app.add_route("/mcp/sse", handle_sse, methods=["GET"])
app.mount("/mcp/messages/", app=sse_transport.handle_post_message)

if __name__ == "__main__":
    print("🚀 Running FastAPI MCP Web Access Server on port 8000...")
    print("MCP SSE Endpoint: http://localhost:8000/mcp/sse")
    print("MCP Post Endpoint: http://localhost:8000/mcp/messages/")
    uvicorn.run(app, host="127.0.0.1", port=8000)
