import asyncio
import os
import sys
from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client
from google import genai
from google.genai import types

# Ensure GEMINI_API_KEY environment variable is set
if not os.environ.get("GEMINI_API_KEY"):
    print("❌ Error: GEMINI_API_KEY environment variable is not set.")
    sys.exit(1)

# Initialize Google Gemini GenAI client
ai = genai.Client()

async def run_single_task(session, task_id, expression, gemini_tools):
    user_prompt = f"Calculate the result of the math expression: {expression} using the calculate tool."
    print(f"[Task {task_id}] 💬 Prompt: \"{user_prompt}\"")
    
    # Run the synchronous generate_content call in a thread pool executor
    # to avoid blocking the asyncio event loop.
    loop = asyncio.get_running_loop()
    try:
        response = await loop.run_in_executor(
            None,
            lambda: ai.models.generate_content(
                model='gemini-2.5-flash',
                contents=user_prompt,
                config=types.GenerateContentConfig(
                    tools=gemini_tools,
                ),
            )
        )
    except Exception as e:
        print(f"[Task {task_id}] ❌ LLM generation error: {e}")
        return

    if response.function_calls:
        call = response.function_calls[0]
        print(f"[Task {task_id}] 🤖 LLM decided to execute tool '{call.name}' with args: {call.args}")
        args_dict = dict(call.args) if call.args else {}

        # Invoke tool call asynchronously through the mcpwatch proxy
        print(f"[Task {task_id}] 📤 Dispatching tool call '{call.name}' through proxy...")
        try:
            tool_result = await session.call_tool(call.name, arguments=args_dict)
            print(f"[Task {task_id}] 📥 Received tool response: {tool_result.content}")
        except Exception as e:
            print(f"[Task {task_id}] ❌ Tool invocation error: {e}")
            return

        # Feed the response back to Gemini to synthesize final text
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
        
        print(f"[Task {task_id}] 🧠 Generating final answer...")
        try:
            final_response = await loop.run_in_executor(
                None,
                lambda: ai.models.generate_content(
                    model='gemini-2.5-flash',
                    contents=history
                )
            )
            print(f"[Task {task_id}] 🤖 Response: \"{final_response.text.strip()}\"")
        except Exception as e:
            print(f"[Task {task_id}] ❌ Final answer generation error: {e}")
    else:
        print(f"[Task {task_id}] 🤖 Response (No tool call): \"{response.text.strip()}\"")

async def run_agent():
    print("🚀 Initializing Python MCP Observability Agent...")
    print("Configuring parallel workload of 10 concurrent agent tasks to stress-test mcpwatch under load.")

    server_cmd = "mcpwatch.exe" if os.name == "nt" else "./mcpwatch"
    wrap_cmd = "test\\testserver.exe" if os.name == "nt" else "./test/testserver"

    # Parameters to spawn mcpwatch wrapping the mock calculator server
    server_params = StdioServerParameters(
        command=server_cmd,
        args=[
            "--wrap", wrap_cmd,
            "--ui", "8080",
            "--gc-percent", "35"
        ]
    )

    print("📡 Connecting to mcpwatch proxy stream...")
    async with stdio_client(server_params) as (read_stream, write_stream):
        async with ClientSession(read_stream, write_stream) as session:
            # 1. Initialize session
            await session.initialize()
            print("✓ Connection established.")

            # 2. Discover tools
            tools_response = await session.list_tools()
            tools = tools_response.tools
            print(f"✓ Discovered {len(tools)} tools.")

            # Map tools to Gemini definitions
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

            # 3. Define concurrent expressions to trigger parallel tool queries
            expressions = [
                "15 + 27",
                "8 * 12",
                "144 / 12",
                "50 - 18",
                "7 * 9",
                "200 / 4",
                "100 + 450",
                "99 - 44",
                "3 * 17",
                "81 / 9"
            ]

            print(f"\n⚡ Spawning {len(expressions)} concurrent agent tasks in parallel...")
            tasks = [
                run_single_task(session, i+1, expr, gemini_tools)
                for i, expr in enumerate(expressions)
            ]
            
            # Execute concurrently
            await asyncio.gather(*tasks)
            print("\n✓ Concurrent complex workload run completed successfully.")

if __name__ == "__main__":
    try:
        asyncio.run(run_agent())
    except KeyboardInterrupt:
        print("\n👋 Test terminated by user.")
    except Exception as e:
        import traceback
        print("\n❌ Error encountered:")
        traceback.print_exc()
