#include <windows.h>
#include <iostream>
#include <string>
#include <vector>
#include <thread>
#include <mutex>
#include <chrono>
#include <queue>
#include <condition_variable>
#include <atomic>
#include "sqlite3.h"

// Simple JSON helper to extract method/id without a full library
struct MessageInfo {
    std::string method;
    std::string id;
    bool isValid = false;
};

struct QueuedMessage {
    std::string direction;
    std::string raw;
};

MessageInfo parseBasicJson(const std::string& json) {
    MessageInfo info;
    size_t methodPos = json.find("\"method\"");
    if (methodPos != std::string::npos) {
        size_t start = json.find("\"", methodPos + 8);
        size_t end = json.find("\"", start + 1);
        if (start != std::string::npos && end != std::string::npos) {
            info.method = json.substr(start + 1, end - start - 1);
            info.isValid = true;
        }
    }
    size_t idPos = json.find("\"id\"");
    if (idPos != std::string::npos) {
        size_t start = json.find(":", idPos + 3);
        size_t end = json.find_first_of(",}", start);
        if (start != std::string::npos && end != std::string::npos) {
            info.id = json.substr(start + 1, end - start - 1);
        }
    }
    return info;
}

// Thread-safe queue for async processing
class MessageQueue {
    std::queue<QueuedMessage> queue;
    std::mutex mutex;
    std::condition_variable cond;
    std::atomic<bool> done{false};

public:
    void push(const std::string& direction, const std::string& raw) {
        {
            std::lock_guard<std::mutex> lock(mutex);
            queue.push({direction, raw});
        }
        cond.notify_one();
    }

    bool pop(QueuedMessage& msg) {
        std::unique_lock<std::mutex> lock(mutex);
        cond.wait(lock, [this] { return !queue.empty() || done; });
        if (queue.empty()) return false;
        msg = std::move(queue.front());
        queue.pop();
        return true;
    }

    void setDone() {
        done = true;
        cond.notify_all();
    }
};

// Forward declaration of Interceptor
class Interceptor {
public:
    virtual void onMessage(const std::string& direction, const std::string& raw) = 0;
};

// Console Interceptor
class ConsoleLogger : public Interceptor {
public:
    void onMessage(const std::string& direction, const std::string& raw) override {
        auto info = parseBasicJson(raw);
        std::string dirIcon = (direction == "OUT") ? "⇠" : "➔";
        std::cout << "[" << direction << "] " << dirIcon << " " << (info.method.empty() ? "Response" : info.method) 
                  << (info.id.empty() ? "" : " [id:" + info.id + "]") << std::endl;
    }
};

// SQLite Interceptor
class SqliteInterceptor : public Interceptor {
    sqlite3* db;
    sqlite3_stmt* stmt;
public:
    SqliteInterceptor(const char* dbPath) {
        if (sqlite3_open(dbPath, &db) == SQLITE_OK) {
            sqlite3_exec(db, "CREATE TABLE IF NOT EXISTS interactions (id INTEGER PRIMARY KEY, direction TEXT, method TEXT, raw TEXT, timestamp DATETIME DEFAULT CURRENT_TIMESTAMP)", NULL, NULL, NULL);
            sqlite3_prepare_v2(db, "INSERT INTO interactions (direction, method, raw) VALUES (?, ?, ?)", -1, &stmt, NULL);
        }
    }
    ~SqliteInterceptor() {
        sqlite3_finalize(stmt);
        sqlite3_close(db);
    }
    void onMessage(const std::string& direction, const std::string& raw) override {
        auto info = parseBasicJson(raw);
        sqlite3_bind_text(stmt, 1, direction.c_str(), -1, SQLITE_TRANSIENT);
        sqlite3_bind_text(stmt, 2, info.method.c_str(), -1, SQLITE_TRANSIENT);
        sqlite3_bind_text(stmt, 3, raw.c_str(), -1, SQLITE_TRANSIENT);
        sqlite3_step(stmt);
        sqlite3_reset(stmt);
    }
};

class Pipeline {
    std::vector<Interceptor*> interceptors;
public:
    void add(Interceptor* i) { interceptors.push_back(i); }
    void process(const std::string& direction, const std::string& raw) {
        for (auto i : interceptors) i->onMessage(direction, raw);
    }
};

void pipeThread(HANDLE hRead, HANDLE hWrite, const std::string& direction, MessageQueue& queue) {
    char buffer[4096];
    DWORD bytesRead, bytesWritten;
    std::string lineBuffer;

    while (ReadFile(hRead, buffer, sizeof(buffer), &bytesRead, NULL) && bytesRead > 0) {
        // CRITICAL: Forward immediately for zero latency
        WriteFile(hWrite, buffer, bytesRead, &bytesWritten, NULL);

        // Process for logging out-of-band
        for (DWORD i = 0; i < bytesRead; ++i) {
            if (buffer[i] == '\n') {
                if (!lineBuffer.empty()) {
                    queue.push(direction, lineBuffer);
                    lineBuffer.clear();
                }
            } else if (buffer[i] != '\r') {
                lineBuffer += buffer[i];
            }
        }
    }
}

void drainThread(MessageQueue& queue, Pipeline& pipeline) {
    QueuedMessage msg;
    while (queue.pop(msg)) {
        pipeline.process(msg.direction, msg.raw);
    }
}

int main(int argc, char* argv[]) {
    if (argc < 2) {
        std::cerr << "Usage: mcpwatch <command> [args...]" << std::endl;
        return 1;
    }

    std::string cmdLine;
    for (int i = 1; i < argc; ++i) {
        cmdLine += argv[i];
        if (i < argc - 1) cmdLine += " ";
    }

    SECURITY_ATTRIBUTES sa = { sizeof(sa), NULL, TRUE };
    HANDLE childInRead, childInWrite;
    HANDLE childOutRead, childOutWrite;

    CreatePipe(&childInRead, &childInWrite, &sa, 0);
    SetHandleInformation(childInWrite, HANDLE_FLAG_INHERIT, 0);

    CreatePipe(&childOutRead, &childOutWrite, &sa, 0);
    SetHandleInformation(childOutRead, HANDLE_FLAG_INHERIT, 0);

    STARTUPINFOA si = { sizeof(si) };
    si.dwFlags = STARTF_USESTDHANDLES;
    si.hStdInput = childInRead;
    si.hStdOutput = childOutWrite;
    si.hStdError = GetStdHandle(STD_ERROR_HANDLE);

    PROCESS_INFORMATION pi = { 0 };

    if (!CreateProcessA(NULL, (LPSTR)cmdLine.c_str(), NULL, NULL, TRUE, 0, NULL, NULL, &si, &pi)) {
        std::cerr << "Failed to start process: " << GetLastError() << std::endl;
        return 1;
    }

    CloseHandle(childInRead);
    CloseHandle(childOutWrite);

    MessageQueue queue;
    Pipeline pipeline;
    ConsoleLogger consoleLogger;
    SqliteInterceptor sqliteInterceptor("mcpwatch.db");
    pipeline.add(&consoleLogger);
    pipeline.add(&sqliteInterceptor);

    // Start background threads
    std::thread inThread(pipeThread, GetStdHandle(STD_INPUT_HANDLE), childInWrite, "IN", std::ref(queue));
    std::thread outThread(pipeThread, childOutRead, GetStdHandle(STD_OUTPUT_HANDLE), "OUT", std::ref(queue));
    std::thread workerThread(drainThread, std::ref(queue), std::ref(pipeline));

    WaitForSingleObject(pi.hProcess, INFINITE);

    // Cleanup
    queue.setDone();
    if (workerThread.joinable()) workerThread.join();

    CloseHandle(childInWrite);
    CloseHandle(childOutRead);
    CloseHandle(pi.hProcess);
    CloseHandle(pi.hThread);

    if (inThread.joinable()) inThread.detach(); 
    if (outThread.joinable()) outThread.detach();

    return 0;
}
