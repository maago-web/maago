package maago

import (
	"testing"

	"github.com/maago-web/maago/internal/rtcore"
)

func TestMessagesCheck(t *testing.T) {
	if err := Messages(rtcore.NewSettings(rtcore.Messages{TooLong: "%d자 이하로"}, rtcore.Messages(DefaultMessages), nil).Msgs).check(); err != nil {
		t.Errorf("a fitting message: %v", err)
	}
	for _, m := range []Messages{{TooLong: "at most %s"}, {Required: "%d required"}, {NoRow: "%s"}, {TooSmall: "%s and %s"}} {
		if err := Messages(rtcore.NewSettings(rtcore.Messages(m), rtcore.Messages(DefaultMessages), nil).Msgs).check(); err == nil {
			t.Errorf("%+v: no error", m)
		}
	}
}
