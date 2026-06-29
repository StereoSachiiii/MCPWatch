import subprocess
import time
import requests
import sys

def main():
    print("🚀 Running Automated eBPF Capture Path Test in WSL...")
    
    # 1. Start testserver_linux
    server_proc = subprocess.Popen(
        ["./test/testserver_linux"],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True
    )
    pid = server_proc.pid
    print(f"✓ Started testserver_linux with PID: {pid}")
    
    # Verify process list and PIDs
    print("=== WSL Process Table ===")
    subprocess.run(["ps", "-ef"])
    print("=========================")
    
    # 2. Start mcpwatch_linux
    print(f"✓ Starting mcpwatch_linux attached to PID: {pid}...")
    mcp_proc = subprocess.Popen(
        ["./mcpwatch_linux", "--pid", str(pid), "--ui", "8083", "--db", "mcpwatch_ebpf_test_db"],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True
    )
    
    # Wait for mcpwatch to attach
    print("⏳ Waiting 3 seconds for eBPF hooks to attach...")
    time.sleep(3)
    
    # 3. Send JSON-RPC requests to the server stdin
    test_requests = [
        '{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"serverInfo":{"name":"calculator-server","version":"1.0.0"}}}',
        '{"jsonrpc":"2.0","id":1,"method":"tools/list"}',
        '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"calculate","arguments":{"expression":"10 + 20"}}}'
    ]
    
    for req in test_requests:
        print(f"📤 Writing to server stdin: {req}")
        server_proc.stdin.write(req + "\n")
        server_proc.stdin.flush()
        
        # Read response
        resp = server_proc.stdout.readline().strip()
        print(f"📥 Received server response: {resp}")
        time.sleep(1.0)
        
    print("⏳ Waiting 2 seconds for database sync...")
    time.sleep(2)
    
    # 4. Query the local mcpwatch api to verify they were captured
    try:
        api_url = "http://localhost:8083/api/interactions"
        resp = requests.get(api_url)
        resp.raise_for_status()
        interactions = resp.json()
        
        print("\n" + "="*50)
        print(f"🔎 VERIFYING CAPTURED INTERACTIONS (Total: {len(interactions)})")
        print("="*50)
        
        captured_methods = [item.get("method") for item in interactions if item.get("method")]
        print(f"Captured Methods: {captured_methods}")
        
        # Check that we captured the 'initialize', 'tools/list', and 'tools/call'
        success = True
        for m in ["initialize", "tools/list", "tools/call"]:
            if m in captured_methods:
                print(f"  ✓ {m} correctly captured in database.")
            else:
                print(f"  ❌ {m} was NOT captured.")
                success = False
                
        if success:
            print("\n🎉 SUCCESS: eBPF Capture Path successfully verified!")
            exit_code = 0
        else:
            print("\n❌ FAILURE: Some interactions were missed by the eBPF tracer.")
            exit_code = 1
            
    except Exception as e:
        print(f"❌ Error querying api: {e}")
        exit_code = 1
        
    finally:
        # Cleanup
        print("🧹 Cleaning up background processes...")
        server_proc.terminate()
        mcp_proc.terminate()
        
        # Read accumulated outputs from mcpwatch
        mcp_stdout, mcp_stderr = mcp_proc.communicate()
        print("\n=== MCPWATCH LINUX STDOUT ===")
        print(mcp_stdout)
        print("=== MCPWATCH LINUX STDERR ===")
        print(mcp_stderr)
        
        try:
            server_proc.wait(timeout=2)
            mcp_proc.wait(timeout=2)
        except:
            pass
        sys.exit(exit_code)

if __name__ == "__main__":
    main()
