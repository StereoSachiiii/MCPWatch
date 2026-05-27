//go:build linux

package transport

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"mcpwatch/internal/engine"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang tracer bpf/tracer.c -- -I../headers

type EBPFHandler struct {
	pid int
}

func NewEBPF(pid int) *EBPFHandler {
	return &EBPFHandler{pid: pid}
}

func (h *EBPFHandler) Type() string {
	return "ebpf"
}

type dataEvent struct {
	Fd      uint32
	Size    uint32
	Payload [4096]byte
}
func (h *EBPFHandler) Start(ctx context.Context, messages chan<- *engine.Message) error {
	log.Printf("[MCPWatch-eBPF] Attaching to PID %d via eBPF tracepoints...\n", h.pid)

	objs := tracerObjects{}
	if err := loadTracerObjects(&objs, nil); err != nil {
		return fmt.Errorf("loading objects: %w", err)
	}
	defer objs.Close()

	key := uint32(0)
	val := uint32(h.pid)
	if err := objs.ConfigPid.Put(&key, &val); err != nil {
		return fmt.Errorf("failed to set target PID: %w", err)
	}

	tpWrite, err := link.Tracepoint("syscalls", "sys_enter_write", objs.TraceWrite, nil)
	if err != nil {
		return fmt.Errorf("opening sys_enter_write tracepoint: %w", err)
	}
	defer tpWrite.Close()

	tpReadEnter, err := link.Tracepoint("syscalls", "sys_enter_read", objs.TraceReadEnter, nil)
	if err != nil {
		return fmt.Errorf("opening sys_enter_read tracepoint: %w", err)
	}
	defer tpReadEnter.Close()

	tpReadExit, err := link.Tracepoint("syscalls", "sys_exit_read", objs.TraceReadExit, nil)
	if err != nil {
		return fmt.Errorf("opening sys_exit_read tracepoint: %w", err)
	}
	defer tpReadExit.Close()

	rd, err := perf.NewReader(objs.Events, 8192*4) // increase reader buffer for large chunks
	if err != nil {
		return fmt.Errorf("creating perf event reader: %w", err)
	}
	defer rd.Close()

	log.Println("[MCPWatch-eBPF] Successfully attached Full-Duplex eBPF hooks. Listening for events...")

	go func() {
		<-ctx.Done()
		rd.Close()
	}()

	var event dataEvent
	
	// Separate buffers for stdin and stdout
	inStream := &streamBuffer{}
	outStream := &streamBuffer{}

	for {
		record, err := rd.Read()
		if err != nil {
			if perf.IsClosed(err) {
				return nil
			}
			log.Printf("reading from perf event reader: %v", err)
			continue
		}

		if record.LostSamples > 0 {
			log.Printf("perf event ring buffer full, dropped %d samples", record.LostSamples)
			continue
		}

		if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
			log.Printf("parsing perf event: %v", err)
			continue
		}

		payloadChunk := event.Payload[:event.Size]
		
		if event.Fd == 0 { // stdin
			inStream.buf.Write(payloadChunk)
			for _, jsonStr := range inStream.extractJSON() {
				if msg := engine.ParseJSONRPC(jsonStr, "IN", h.Type()); msg != nil {
					messages <- msg
				}
			}
		} else if event.Fd == 1 { // stdout
			outStream.buf.Write(payloadChunk)
			for _, jsonStr := range outStream.extractJSON() {
				if msg := engine.ParseJSONRPC(jsonStr, "OUT", h.Type()); msg != nil {
					messages <- msg
				}
			}
		}
	}
}
