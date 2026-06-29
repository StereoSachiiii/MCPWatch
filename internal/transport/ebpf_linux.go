//go:build linux

package transport

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"mcpwatch/internal/engine"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang tracer bpf/tracer.c -- -I../headers

type EBPFHandler struct {
	pid    int
	parser engine.Parser
}

func NewEBPF(pid int, parser engine.Parser) *EBPFHandler {
	return &EBPFHandler{pid: pid, parser: parser}
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
	slog.Info("Attaching to PID via eBPF tracepoints", "pid", h.pid)

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

	rd, err := perf.NewReader(objs.Events, 8192*4) 
	if err != nil {
		return fmt.Errorf("creating perf event reader: %w", err)
	}
	defer rd.Close()

	slog.Info("Successfully attached Full-Duplex eBPF hooks. Listening for events...")

	go func() {
		<-ctx.Done()
		rd.Close()
	}()

	var event dataEvent

	inStream := &streamBuffer{}
	outStream := &streamBuffer{}
	errStream := &streamBuffer{}

	for {
		record, err := rd.Read()
		if err != nil {
			if errors.Is(err, perf.ErrClosed) {
				return nil
			}
			slog.Error("reading from perf event reader", "error", err)
			continue
		}

		if record.LostSamples > 0 {
			slog.Warn("perf event ring buffer full", "lost_samples", record.LostSamples)
			continue
		}

		if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
			slog.Error("parsing perf event", "error", err)
			continue
		}

		payloadChunk := event.Payload[:event.Size]

		if event.Fd == 0 { 
			inStream.buf.Write(payloadChunk)
			for _, jsonStr := range inStream.extractJSON() {
				if msg := h.parser.Parse(jsonStr, "IN", h.Type()); msg != nil {
					select {
					case messages <- msg:
					default:
					}
				}
			}
		} else if event.Fd == 1 { 
			outStream.buf.Write(payloadChunk)
			for _, jsonStr := range outStream.extractJSON() {
				if msg := h.parser.Parse(jsonStr, "OUT", h.Type()); msg != nil {
					select {
					case messages <- msg:
					default:
					}
				}
			}
		} else if event.Fd == 2 {
			errStream.buf.Write(payloadChunk)
			for _, line := range errStream.extractLines() {
				select {
				case messages <- &engine.Message{
					Timestamp:     time.Now(),
					Transport:     h.Type(),
					Direction:     "ERR",
					MsgType:       engine.MsgTypeStderr,
					Raw:           line,
					SizeBytes:     int64(len(line)),
					TokenEstimate: int64(len(line)) / 4,
				}:
				default:
				}
			}
		}
	}
}
