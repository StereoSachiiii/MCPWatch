mcpwatch -> observation proxy for mcp servers

features ->
intercepts stdin and stdout -> logs json-rpc traffic
stores data in sqlite -> mcpwatch.db
web dashboard -> live view of interactions

setup ->
go build -o mcpwatch.exe

usage ->
mcpwatch.exe --wrap "your server command"
example -> mcpwatch.exe --wrap "node server.js"

dashboard ->
localhost:8080 -> see all traffic
