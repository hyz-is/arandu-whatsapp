package authz

import (
	"testing"

	"github.com/arandu-io/framework/security"
)

// TestWritesNamesEveryReadingActionAndNothingElse pins the closed set of
// actions whose Grant may not write. An action added to the package has to
// be placed in this table, so it is decided rather than inherited.
func TestWritesNamesEveryReadingActionAndNothingElse(t *testing.T) {
	reads := map[security.Action]bool{
		ActionInstanceList: true,
		ActionInstanceView: true,
		ActionWebhookView:  true,
	}
	writes := []security.Action{
		ActionInstanceCreate, ActionInstanceUpdate, ActionInstanceDelete,
		ActionConnectionPair, ActionConnectionView, ActionConnectionLogout,
		ActionWebhookSet,
		ActionMessageSend, ActionMessageList, ActionMessageRead, ActionMessageDelete,
		ActionMessageEdit, ActionMessageMediaDownload,
		ActionContactCheck, ActionChatArchive, ActionProfilePictureView, ActionCallReject,
		ActionGroupCreate, ActionGroupPictureUpdate, ActionGroupInviteView,
		ActionGroupInviteRevoke, ActionGroupParticipantUpdate, ActionGroupLeave,
		ActionRuntime, ActionRuntimeDiscover,
	}
	for action := range reads {
		if Writes(action) {
			t.Errorf("Writes(%q) = true, want false: the action only reads", action)
		}
	}
	for _, action := range writes {
		if !Writes(action) {
			t.Errorf("Writes(%q) = false, want true: the action may write", action)
		}
	}
	if got := len(reads) + len(writes); got != 28 {
		t.Errorf("the table covers %d actions, want 28: place a new action in it", got)
	}
}
