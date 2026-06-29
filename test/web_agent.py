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

                # 3. Dynamic Agent Loop
                user_prompt = (
                    "Find the name of the original designers of the Go programming language from "
                    "http://localhost:8081/mock/go using your tool. "
                    "Then, select one of the designers (Rob Pike or Ken Thompson), fetch their individual "
                    "mock webpage (use the URL provided in the page), and determine their date of birth. "
                    "Provide a final summary identifying the designers and the birth date of the selected designer."
                )
                print(f'\n💬 User Agent Goal: "{user_prompt}"')

                # Map MCP tools to Gemini declarations
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

                # Initialize chat history with the user prompt
                messages = [
                    types.Content(role="user", parts=[types.Part.from_text(text=user_prompt)])
                ]

                max_turns = 5
                turn = 0
                while turn < max_turns:
                    turn += 1
                    print(f"\n🧠 [Turn {turn}] Asking LLM model...")

                    # Run blocking model generate in executor to keep event loop responsive
                    loop = asyncio.get_running_loop()
                    response = await loop.run_in_executor(
                        None,
                        lambda: ai.models.generate_content(
                            model='gemini-2.5-flash',
                            contents=messages,
                            config=types.GenerateContentConfig(
                                tools=gemini_tools,
                            ),
                        )
                    )

                    # Append model response to message history
                    messages.append(response.candidates[0].content)

                    if response.function_calls:
                        # Iterate through each function call requested by the model in this turn
                        for call in response.function_calls:
                            print(f"🤖 Agent decided to execute tool '{call.name}' with arguments: {call.args}")
                            args_dict = dict(call.args) if call.args else {}

                            # Dispatch tool call through mcpwatch proxy
                            print(f"📤 Dispatching tool call '{call.name}' through proxy...")
                            tool_result = await session.call_tool(call.name, arguments=args_dict)
                            
                            # Parse out text responses
                            results_list = []
                            for block in tool_result.content:
                                if hasattr(block, "text"):
                                    results_list.append(block.text)
                                else:
                                    results_list.append(str(block))
                            raw_text = "\n".join(results_list)
                            print(f"📥 Received tool response of size: {len(raw_text)} characters.")

                            # Append function response to the chat history
                            messages.append(
                                types.Content(
                                    role="user",
                                    parts=[
                                        types.Part.from_function_response(
                                            name=call.name,
                                            response={"result": raw_text}
                                        )
                                    ]
                                )
                            )
                    else:
                        # No function calls means the agent is done and has synthesized the final answer
                        print(f"\n🤖 Final Agent Response:\n{response.text.strip()}\n")
                        break
                else:
                    print("⚠️ Reached maximum number of turns without completing goal.")
                    
    except Exception as e:
        import traceback
        print("\n❌ Error encountered:")
        traceback.print_exc()

if __name__ == "__main__":
    try:
        asyncio.run(run_web_agent())
    except KeyboardInterrupt:
        print("\n👋 Test terminated by user.")
