package whatsapp

import (
	"testing"

	"github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

// A connect call for an instance that is already pairing answers the QR code
// the pairing shows now, so a screen that polls keeps up with WhatsApp's
// rotation; before the first code arrives it answers connecting, with none.
func TestAConnectCallDuringPairingAnswersTheCurrentQRCode(t *testing.T) {
	s := &Service{pairings: newPairingManager()}
	session := &pairingSession{}
	if !s.pairings.add("42", session) {
		t.Fatal("the pairing session was not registered")
	}

	got := s.pairingInProgress("42", "atendimento-1")
	if !got.AlreadyConnecting || got.Base64 != "" || got.ConnectionStatus != string(types.InstanceConnectionStatusConnecting) {
		t.Fatalf("before the first code: %+v", got)
	}

	session.setCurrentQR(QRCodeConnectionResult{Count: 1, Code: "first", Base64: "data:image/png;base64,AAA"})
	session.setCurrentQR(QRCodeConnectionResult{Count: 2, Code: "second", Base64: "data:image/png;base64,BBB"})
	got = s.pairingInProgress("42", "atendimento-1")
	if got.Count != 2 || got.Code != "second" || got.Base64 != "data:image/png;base64,BBB" || !got.AlreadyConnecting {
		t.Fatalf("during pairing the call answered %+v, want the second code", got)
	}
	if got.ConnectionStatus != string(types.InstanceConnectionStatusQRCode) || got.InstanceName != "atendimento-1" {
		t.Fatalf("status %q, name %q", got.ConnectionStatus, got.InstanceName)
	}

	if other := s.pairingInProgress("7", "outro"); other.Base64 != "" {
		t.Fatalf("an instance with no pairing answered a code: %+v", other)
	}
}
