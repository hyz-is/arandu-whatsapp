package whatsapp

import (
	"testing"

	hlog "github.com/arandu-io/hesape/log"
)

func TestWhatsmeowLoggerMapsDetailedOutputToDebug(t *testing.T) {
	logger, records := hlog.Capture()
	adapter := NewWhatsmeowLogger(logger).Sub("socket")

	adapter.Debugf("frame %d", 7)

	if records.Len() != 1 {
		t.Fatalf("captured %d records, want 1", records.Len())
	}
	line := records.All()[0]
	if line.Level != hlog.LevelDebug || line.Message != "frame 7" {
		t.Fatalf("unexpected record: %#v", line)
	}
	if line.Attrs["component"] != "whatsmeow" || line.Attrs["module"] != "socket" {
		t.Fatalf("unexpected attributes: %#v", line.Attrs)
	}
}
