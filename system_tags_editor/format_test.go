package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// parse 是测试里的简写：把字符串解析成有序结构。
func parse(t *testing.T, s string) any {
	t.Helper()
	v, err := ParseOrdered(strings.NewReader(s))
	if err != nil {
		t.Fatalf("ParseOrdered(%q): %v", s, err)
	}
	return v
}

// 键序是这个工具存在的理由。普通 encoding/json 会把键排序，
// 那样每次保存都会重排整个文件，diff 没法看。
func TestParseOrderedKeepsKeyOrder(t *testing.T) {
	v := parse(t, `{"zebra":1,"apple":2,"mango":3}`)
	om, ok := v.(*OrderedMap)
	if !ok {
		t.Fatalf("got %T, want *OrderedMap", v)
	}
	want := []string{"zebra", "apple", "mango"}
	if len(om.Keys) != len(want) {
		t.Fatalf("keys = %v, want %v", om.Keys, want)
	}
	for i, k := range want {
		if om.Keys[i] != k {
			t.Fatalf("key[%d] = %q, want %q（全序 %v）", i, om.Keys[i], k, om.Keys)
		}
	}
}

func TestParseOrderedNestedKeepsOrder(t *testing.T) {
	v := parse(t, `{"outer":{"c":1,"a":2},"arr":[{"y":1,"b":2}]}`)
	om := v.(*OrderedMap)
	outer := om.M["outer"].(*OrderedMap)
	if strings.Join(outer.Keys, ",") != "c,a" {
		t.Fatalf("nested keys = %v, want [c a]", outer.Keys)
	}
	arr := om.M["arr"].([]any)
	inner := arr[0].(*OrderedMap)
	if strings.Join(inner.Keys, ",") != "y,b" {
		t.Fatalf("array elem keys = %v, want [y b]", inner.Keys)
	}
}

// 同一个输入必须产出同样的字节，否则「保存后 diff 干净」无从谈起。
func TestDumpCompactDeterministic(t *testing.T) {
	src := `{"version":1,"trees":{"qc":[{"code":"a","name":"A"}],"job":[]}}`
	first := DumpCompact(parse(t, src))
	second := DumpCompact(parse(t, src))
	if first != second {
		t.Fatalf("两次序列化结果不同:\n%q\n%q", first, second)
	}
}

func TestDumpCompactEndsWithSingleNewline(t *testing.T) {
	out := DumpCompact(parse(t, `{"a":1}`))
	if !strings.HasSuffix(out, "\n") || strings.HasSuffix(out, "\n\n") {
		t.Fatalf("输出应以单个换行结尾，实际 %q", out)
	}
}

// 空的编辑态值不能写回仓库，否则产生脏 diff。
func TestCleanTagDataDropsEmpty(t *testing.T) {
	v := parse(t, `{"code":"x","error_grade":"  ","tenant_tag":[],"device_scope":{"job_types":[],"device_models":["RD001"]}}`)
	CleanTagData(v)
	om := v.(*OrderedMap)

	if _, ok := om.M["error_grade"]; ok {
		t.Error("空 error_grade 应被删掉")
	}
	if _, ok := om.M["tenant_tag"]; ok {
		t.Error("空 tenant_tag 应被删掉")
	}
	ds := om.M["device_scope"].(*OrderedMap)
	if _, ok := ds.M["job_types"]; ok {
		t.Error("device_scope 里的空数组应被删掉")
	}
	if _, ok := ds.M["device_models"]; !ok {
		t.Error("非空的 device_models 不该被删")
	}
	if len(ds.Keys) != 1 || ds.Keys[0] != "device_models" {
		t.Errorf("device_scope keys = %v, want [device_models]", ds.Keys)
	}
}

// device_scope 内部全空时，外层也该被删掉，而不是留一个空对象。
func TestCleanTagDataDropsEmptyScope(t *testing.T) {
	v := parse(t, `{"code":"x","device_scope":{"job_types":[]}}`)
	CleanTagData(v)
	om := v.(*OrderedMap)
	if _, ok := om.M["device_scope"]; ok {
		t.Errorf("空 device_scope 应被删掉，实际 keys=%v", om.Keys)
	}
}

// 非空值一个都不能丢——清理函数最容易写过头。
func TestCleanTagDataKeepsContent(t *testing.T) {
	src := `{"code":"x","error_grade":"A","tenant_tag":["BODYLESS"],"device_scope":{"job_types":[1,2]}}`
	v := parse(t, src)
	CleanTagData(v)
	if !RoundTripOK(DumpCompact(v), parse(t, src)) {
		t.Fatalf("清理后语义变了:\n原 %s\n后 %s", src, DumpCompact(v))
	}
}

// device_scope 键序按 scopeKeyOrder 规范化：body_classes / device_models / job_types。
// 已拍板：这是有意规范，以后所有文件都按这个顺序。
func TestCanonicalScopeOrder(t *testing.T) {
	// 故意打乱输入顺序
	v := parse(t, `{"job_types":[1],"body_classes":["no_body"],"device_models":["RD001"]}`)
	out := canonicalScope(v).(*OrderedMap)
	want := []string{"body_classes", "device_models", "job_types"}
	if strings.Join(out.Keys, ",") != strings.Join(want, ",") {
		t.Fatalf("canonicalScope keys = %v, want %v", out.Keys, want)
	}
}

// 未知键不允许被丢掉：仓库将来加新键时，编辑一次不能把它吃掉。
func TestCanonicalScopeKeepsUnknownKeys(t *testing.T) {
	v := parse(t, `{"job_types":[1],"future_key":"x"}`)
	out := canonicalScope(v).(*OrderedMap)
	found := false
	for _, k := range out.Keys {
		if k == "future_key" {
			found = true
		}
	}
	if !found {
		t.Fatalf("未知键被丢掉了: %v", out.Keys)
	}
	// 未知键排在已知键之后
	if out.Keys[len(out.Keys)-1] != "future_key" {
		t.Fatalf("未知键应排在末尾，实际 %v", out.Keys)
	}
}

func TestRoundTripOKRejectsBroken(t *testing.T) {
	orig := parse(t, `{"a":1}`)
	if RoundTripOK("", orig) {
		t.Error("空文本不该算往返成功")
	}
	if RoundTripOK("{not json", orig) {
		t.Error("坏 JSON 不该算往返成功")
	}
}

// 用真实仓库文件往返。文件不在本机时跳过——所以合成用例必须自己站得住。
func TestRoundTripRealSystemTags(t *testing.T) {
	path := filepath.Join("..", "..", "data-service", "configs", "tag", "system_tags.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("真实文件不在本机: %v", err)
	}
	orig, err := ParseOrdered(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("解析真实文件: %v", err)
	}
	CleanTagData(orig)
	text := DumpCompact(orig)
	if !RoundTripOK(text, orig) {
		t.Fatal("真实文件往返后语义变了")
	}

	// 已知格式漂移：device_scope 键序被规范化，行宽拆行也可能差几处。
	// 这里不断言「字节完全相同」——那会在 canonicalScope 有意生效时误报。
	// 只断言：漂移量小、且重复保存不再产生新漂移（幂等）。
	if delta := len(text) - len(raw); delta > 64 || delta < -64 {
		t.Errorf("格式漂移过大: %d 字节（原 %d → %d）", delta, len(raw), len(text))
	}
	again := DumpCompact(parse(t, text))
	if again != text {
		t.Error("第二次保存产生了新漂移——保存不幂等，每次都会改文件")
	}
}
