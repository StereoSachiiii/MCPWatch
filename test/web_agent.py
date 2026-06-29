import asyncio
import os
import sys
from mcp import ClientSession
from mcp.client.sse import sse_client
from google import genai
from google.genai import types

# Ensure GEMINI_API_KEY environment variable is set
if not os.environ.get("GEMINI_API_KEY"):
    print("❌ Error: GEMINI_API_KEY environment variable is not set.")
    sys.exit(1)

# Initialize Google Gemini GenAI client
ai = genai.Client()

async def run_web_agent():
    print("🚀 Initializing Python MCP Web Observability Agent...")
    print("Connecting to mcpwatch proxy on port 8081 (which proxies to the FastAPI MCP server on 8000)...")

    # Connect to mcpwatch proxy SSE endpoint
    # mcpwatch must be running in --proxy mode listening on 8081
    try:
        async with sse_client("http://localhost:8081/mcp/sse") as (read_stream, write_stream):
            async with ClientSession(read_stream, write_stream) as session:
                # 1. Initialize session
                await session.initialize()
                print("✓ Connection established.")

                # 2. Discover tools
                tools_response = await session.list_tools()
                tools = tools_response.tools
                print(f"✓ Discovered tools: {[t.name for t in tools]}")

                # 3. Ask Gemini to summarize a web page
                user_prompt = "Fetch the webpage http://example.com and summarize what the domain is reserved for in 2 sentences."
                print(f'\n💬 User Agent Goal: "{user_prompt}"')

                # Map to Gemini definitions
                gemini_tools = []
                for t in tools:
                    gemini_tools.append(
                        types.Tool(
                            function_declarations=[
                                types.FunctionDeclaration(
                                    name=t.name,
                                    description=t.description,
                                    parameters=t.inputSchema
                                )
                            ]
                        )
                    )

                print("🧠 Asking LLM model...")
                response = ai.models.generate_content(
                    model='gemini-2.5-flash',
                    contents=user_prompt,
                    config=types.GenerateContentConfig(
                        tools=gemini_tools,
                    ),
                )

                # Check if Gemini wants to call a tool
                if response.function_calls:
                    call = response.function_calls[0]
                    print(f"🤖 Agent decided to execute tool '{call.name}' with arguments: {call.args}")

                    args_dict = dict(call.args) if call.args else {}

                    # 4. Invoke tool call via proxy
                    print("📤 Dispatching tool call through proxy...")
                    tool_result = await session.call_tool(call.name, arguments=args_dict)
                    print(f"📥 Received tool response.")

                    # 5. Send result back to Gemini
                    history = [
                        types.Content(role="user", parts=[types.Part.from_text(text=user_prompt)]),
                        response.candidates[0].content,
                        types.Content(
                            role="user",
                            parts=[
                                types.Part.from_function_response(
                                    name=call.name,
                                    response={"result": tool_result.content}
                                )
                            ]
                        )
                    ]

                    print("🧠 Generating final response text...")
                    final_response = ai.models.generate_content(
                        model='gemini-2.5-flash',
                        contents=history
                    )
                    print(f"\n🤖 Final Agent Response: \"{final_response.text.strip()}\"")
                else:
                    print(f"\n🤖 Final Agent Response (No tool calls): \"{response.text.strip()}\"")
                    
    except Exception as e:
        import traceback
        print("\n❌ Error encountered:")
        traceback.print_exc()

if __name__ == "__main__":
    try:
        asyncio.run(run_web_agent())
    except KeyboardInterrupt:
        print("\n👋 Test terminated by user.")
