package attendance

import (
	"errors"
	"strings"
	"testing"
)

func TestFleetManagedBlocksUseWhiteAndPreserveExistingText(t *testing.T) {
	old := "<font color=\"#ffff0000\">已有指令</font><br>[GNV NOTICE]<br>旧公告<br>[/GNV NOTICE]<br>[GNV PAP] 未发放 [/GNV PAP]<br>末尾"
	notice, err := mergeFleetNotice(old, "新公告<br>星系：Jita")
	if err != nil {
		t.Fatal(err)
	}
	got, err := mergeFleetPAP(notice, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "<font color=\"#ffff0000\">已有指令</font><br>") || !strings.HasSuffix(got, "<br>末尾") {
		t.Fatalf("unrelated content changed: %q", got)
	}
	for _, want := range []string{`<font color="#ffffffff">[GNV NOTICE]<br>新公告<br>星系：Jita<br>[/GNV NOTICE]</font>`, `<font color="#ffffffff">[GNV PAP] 已发放 · 每角色 2 PAP [/GNV PAP]</font>`} {
		if !strings.Contains(got, want) {
			t.Fatalf("managed block missing explicit white: %q", got)
		}
	}
	again, err := mergeFleetNotice(got, "新公告<br>星系：Jita")
	if err != nil {
		t.Fatal(err)
	}
	again, err = mergeFleetPAP(again, 2)
	if err != nil || again != got {
		t.Fatalf("repeated sync changed formatting: %q, %v", again, err)
	}
}

func TestFleetManagedBlocksRejectAmbiguousMarkers(t *testing.T) {
	for _, old := range []string{
		"[GNV NOTICE] 不完整",
		"[/GNV NOTICE] 反向 [GNV NOTICE]",
		"[GNV NOTICE] 一 [/GNV NOTICE] [GNV NOTICE] 二 [/GNV NOTICE]",
	} {
		if _, err := mergeFleetNotice(old, "公告"); !errors.Is(err, ErrConflict) {
			t.Fatalf("accepted ambiguous marker %q: %v", old, err)
		}
	}
	if _, err := mergeFleetPAP("[GNV PAP] 不完整", 0); !errors.Is(err, ErrConflict) {
		t.Fatalf("accepted incomplete PAP marker: %v", err)
	}
}
