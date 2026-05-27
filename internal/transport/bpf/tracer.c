//go:build ignore

#include <linux/bpf.h>
#include <linux/ptrace.h>
#include <bpf/bpf_helpers.h>

char __license[] SEC("license") = "Dual MIT/GPL";

// Struct sent to user-space
struct data_event {
    __u32 fd;
    __u32 size;
    __u8 payload[4096]; // Increased to 4KB chunks
};

// Perf event array to send data to Go
struct {
    __uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
    __uint(key_size, sizeof(__u32));
    __uint(value_size, sizeof(__u32));
} events SEC(".maps");

// Map to hold the target PID we want to trace
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, __u32);
} config_pid SEC(".maps");

// Per-CPU array to hold the event structure (avoids 512B stack limit)
struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct data_event);
} event_buf_map SEC(".maps");

// Map to store buffer pointers between sys_enter_read and sys_exit_read
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1024);
    __type(key, __u64); // pid_tgid
    __type(value, const char *);
} read_args SEC(".maps");

struct sys_enter_write_args {
    unsigned long long unused;
    int syscall_nr;
    unsigned int fd;
    const char *buf;
    size_t count;
};

SEC("tracepoint/syscalls/sys_enter_write")
int trace_write(struct sys_enter_write_args *ctx) {
    __u32 key = 0;
    __u32 *target = bpf_map_lookup_elem(&config_pid, &key);
    if (!target) return 0;

    __u64 id = bpf_get_current_pid_tgid();
    __u32 pid = id >> 32;

    if (pid != *target) return 0;
    if (ctx->fd != 1) return 0; // stdout only

    struct data_event *e = bpf_map_lookup_elem(&event_buf_map, &key);
    if (!e) return 0;

    e->fd = ctx->fd;
    e->size = ctx->count;
    if (e->size > sizeof(e->payload)) {
        e->size = sizeof(e->payload);
    }
    
    bpf_probe_read_user(e->payload, e->size, ctx->buf);
    bpf_perf_event_output(ctx, &events, BPF_F_CURRENT_CPU, e, sizeof(*e));
    
    return 0;
}

struct sys_enter_read_args {
    unsigned long long unused;
    int syscall_nr;
    unsigned int fd;
    const char *buf;
    size_t count;
};

SEC("tracepoint/syscalls/sys_enter_read")
int trace_read_enter(struct sys_enter_read_args *ctx) {
    __u32 key = 0;
    __u32 *target = bpf_map_lookup_elem(&config_pid, &key);
    if (!target) return 0;

    __u64 id = bpf_get_current_pid_tgid();
    if ((id >> 32) != *target) return 0;
    if (ctx->fd != 0) return 0; // stdin only

    const char *buf = ctx->buf;
    bpf_map_update_elem(&read_args, &id, &buf, BPF_ANY);
    return 0;
}

struct sys_exit_read_args {
    unsigned long long unused;
    int syscall_nr;
    long ret;
};

SEC("tracepoint/syscalls/sys_exit_read")
int trace_read_exit(struct sys_exit_read_args *ctx) {
    __u64 id = bpf_get_current_pid_tgid();
    const char **buf_ptr = bpf_map_lookup_elem(&read_args, &id);
    if (!buf_ptr) return 0;
    
    const char *buf = *buf_ptr;
    bpf_map_delete_elem(&read_args, &id);

    if (ctx->ret <= 0) return 0;

    __u32 zero = 0;
    struct data_event *e = bpf_map_lookup_elem(&event_buf_map, &zero);
    if (!e) return 0;

    e->fd = 0;
    e->size = ctx->ret;
    if (e->size > sizeof(e->payload)) {
        e->size = sizeof(e->payload);
    }

    bpf_probe_read_user(e->payload, e->size, buf);
    bpf_perf_event_output(ctx, &events, BPF_F_CURRENT_CPU, e, sizeof(*e));

    return 0;
}
