package knowledge

import (
	"strings"
	"testing"
)

func TestSensitiveContent(t *testing.T) {
	for _, text := range []string{"api_key=abcd1234", "密码：不要共享这个密码", "postgres://user:credential@db/app", "-----BEGIN RSA PRIVATE KEY-----", "Bearer abcdefghijklmnopqrstuvwxyz", "联系 13812345678"} {
		if !Sensitive(text) {
			t.Errorf("missed sensitive input: %q", text)
		}
	}
	for _, text := range []string{"Docker DNS 排查", "API Key 不应写入文档", "用户已确认重启后恢复"} {
		if Sensitive(text) {
			t.Errorf("false positive: %q", text)
		}
	}
}

func TestStrictModelJSON(t *testing.T) {
	for _, raw := range []string{"```json\n{}\n```", `{"should_save":true,"unexpected":1}`, `{} {}`, `{"should_save":"true"}`, strings.Repeat("x", 1024*1024+1)} {
		var e Extraction
		if Decode(raw, &e) == nil {
			t.Errorf("accepted invalid output %q", raw[:min(len(raw), 50)])
		}
	}
	var e Extraction
	if err := Decode(`{"should_save":false,"action":"IGNORE"}`, &e); err != nil {
		t.Fatal(err)
	}
	if err := Validate(e); err != nil {
		t.Fatal(err)
	}
}

func TestIncrementalEditsPreserveUnrelatedContent(t *testing.T) {
	before := "# 操作流程\n\n![拓扑](assets/network.png)\n\n旧地址\n\n[参考](https://example.test)\n保留其他配置。"
	after, err := Apply(before, "md", Extraction{Action: "UPDATE", OldText: "旧地址", NewText: "新地址"})
	if err != nil {
		t.Fatal(err)
	}
	if after != strings.Replace(before, "旧地址", "新地址", 1) {
		t.Fatal("unrelated content changed")
	}
	for _, e := range []Extraction{{Action: "UPDATE", OldText: "不存在", NewText: "替换"}, {Action: "UPDATE", OldText: "", NewText: "覆盖"}, {Action: "UPDATE", OldText: before, NewText: "整篇覆盖"}, {Action: "APPEND", Content: "旧地址"}} {
		if _, err := Apply(before, "md", e); err == nil {
			t.Fatal("unsafe update accepted")
		}
	}
	if _, err := Apply("<p>原文</p>", "html", Extraction{Action: "APPEND", Content: "Markdown 补充"}); err == nil {
		t.Fatal("HTML was rewritten as markdown")
	}
}

func TestProposalValidation(t *testing.T) {
	valid := Extraction{ShouldSave: true, Title: "部署经验", Content: "容器恢复后确认服务正常", Action: "CREATE", Reason: "用户提供", RiskLevel: "low", VerificationStatus: "user_provided", SourceMessageIDs: []string{"message"}}
	if err := Validate(valid); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Extraction){func(e *Extraction) { e.Action = "DELETE" }, func(e *Extraction) { e.RiskLevel = "safe" }, func(e *Extraction) { e.VerificationStatus = "confirmed_by_model" }, func(e *Extraction) { e.SourceMessageIDs = nil }, func(e *Extraction) { e.Content = "password=secret123" }, func(e *Extraction) { e.Action = "UPDATE" }} {
		e := valid
		mutate(&e)
		if Validate(e) == nil {
			t.Fatal("invalid proposal accepted")
		}
	}
}
