package compile

import (
	"strings"
	"testing"

	"github.com/buchk/TypeFerence/go/internal/jsonx"
	"github.com/buchk/TypeFerence/go/internal/resolve"
)

func TestSkillValueOmitsVariantsWhenUnimodal(t *testing.T) {
	out := jsonx.Indented(skillValue(resolve.ResolvedSkill{DispatchName: "a.b", Instructions: "x"}))
	if strings.Contains(out, "variants") {
		t.Errorf("a unimodal skill must not emit a variants member:\n%s", out)
	}
}

func TestSkillValueEmitsVariantsWhenMultimodal(t *testing.T) {
	out := jsonx.Indented(skillValue(resolve.ResolvedSkill{
		DispatchName: "a.b",
		Instructions: "strict",
		Variants:     map[string]string{"pipeline": "strict", "manual": "explain"},
	}))
	if !strings.Contains(out, `"variants"`) {
		t.Errorf("a multimodal skill must emit a variants member:\n%s", out)
	}
	if !strings.Contains(out, "explain") {
		t.Errorf("variants member must contain each mode's instructions:\n%s", out)
	}
}
