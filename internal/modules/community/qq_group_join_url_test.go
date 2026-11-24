package community

import (
	"testing"

	"glorynavy.local/seat/internal/testutil"
)

func TestQQGroupJoinURLSettingsAndMemberOptions(t *testing.T) {
	service := New(testutil.Database(t))
	ctx := t.Context()
	const invite = "https://qm.qq.com/q/ExampleInvite"
	settings := []QQGroupSetting{
		{GroupOpenID: "enabled-group", Enabled: true, JoinURL: " " + invite + " "},
		{GroupOpenID: "disabled-group", JoinURL: invite},
	}
	saved, err := service.SaveQQGroupSettings(ctx, settings)
	if err != nil || len(saved.Items) != 2 || saved.Items[0].JoinURL != invite {
		t.Fatalf("save group invite URL: %+v, %v", saved, err)
	}
	for _, invalid := range []string{
		"http://qm.qq.com/q/ExampleInvite",
		"https://qm.qq.com.evil.invalid/q/ExampleInvite",
		"https://someone@qm.qq.com/q/ExampleInvite",
		"https://qm.qq.com/q/../redirect",
		"https://qm.qq.com/q/",
	} {
		if _, err = service.SaveQQGroupSettings(ctx, []QQGroupSetting{{GroupOpenID: "enabled-group", Enabled: true, JoinURL: invalid}}); err == nil {
			t.Errorf("accepted invalid group invite URL %q", invalid)
		}
	}
	options, err := service.ListQQGroupOptions(ctx)
	if err != nil || len(options.Items) != 1 || options.Items[0].GroupOpenID != "enabled-group" || options.Items[0].JoinURL != invite {
		t.Fatalf("member group options: %+v, %v", options, err)
	}
	if _, err = service.SaveQQGroupSettings(ctx, []QQGroupSetting{{GroupOpenID: "enabled-group", Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	options, err = service.ListQQGroupOptions(ctx)
	if err != nil || len(options.Items) != 1 || options.Items[0].JoinURL != "" {
		t.Fatalf("cleared group invite URL: %+v, %v", options, err)
	}
}
