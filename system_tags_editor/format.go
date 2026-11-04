package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
)

// OrderedMap 保留 JSON 对象键顺序的解析结果。
type OrderedMap struct {
	Keys []string
	M    map[string]any
}

func (o *OrderedMap) Has(k string) bool {
	_, ok := o.M[k]
	return ok
}

// MarshalJSON 按原始键顺序序列化。
func (o *OrderedMap) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range o.Keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		vb, err := json.Marshal(o.M[k])
		if err != nil {
			return nil, err
		}
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// ParseOrdered 解析 JSON 并保留键顺序；数字保留原始字面量（json.Number）。
func ParseOrdered(r io.Reader) (any, error) {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	v, err := parseTok(dec)
	if err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, fmt.Errorf("trailing data after JSON value")
	}
	return v, nil
}

func parseTok(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); ok {
		switch d {
		case '{':
			o := &OrderedMap{M: map[string]any{}}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, ok := kt.(string)
				if !ok {
					return nil, fmt.Errorf("non-string object key: %v", kt)
				}
				val, err := parseTok(dec)
				if err != nil {
					return nil, err
				}
				if !o.Has(key) {
					o.Keys = append(o.Keys, key)
				}
				o.M[key] = val
			}
			if _, err := dec.Token(); err != nil { // 消费 '}'
				return nil, err
			}
			return o, nil
		case '[':
			var a []any
			for dec.More() {
				v, err := parseTok(dec)
				if err != nil {
					return nil, err
				}
				a = append(a, v)
			}
			if _, err := dec.Token(); err != nil { // 消费 ']'
				return nil, err
			}
			return a, nil
		default:
			return nil, fmt.Errorf("unexpected delimiter %v", d)
		}
	}
	return tok, nil // string / json.Number / bool / nil
}

// ---------- compact serializer（与仓库手写风格一致） ----------

func isLeaf(v any) bool {
	o, ok := v.(*OrderedMap)
	return ok && o.Has("code") && !o.Has("children")
}

func marshalKey(k string) string {
	b, _ := json.Marshal(k)
	return string(b)
}

// inline 单行渲染：分隔符统一 ", " / ": "。
func inline(v any) string {
	switch t := v.(type) {
	case *OrderedMap:
		return inlineScope(t)
	case []any:
		parts := make([]string, len(t))
		for i, e := range t {
			parts[i] = inline(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case string:
		return marshalKey(t)
	case nil:
		return "null"
	default: // json.Number / bool
		return fmt.Sprintf("%v", t)
	}
}

func inlineScope(o *OrderedMap) string {
	if len(o.Keys) == 0 {
		return "{}"
	}
	parts := make([]string, len(o.Keys))
	for i, k := range o.Keys {
		parts[i] = marshalKey(k) + ": " + inline(o.M[k])
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

// scopeKeyOrder device_scope 键的规范输出顺序（与仓库手写风格一致）。
var scopeKeyOrder = []string{"body_classes", "device_models", "job_types"}

// canonicalScope 返回键按规范序重排的 device_scope 副本，避免编辑态键序漂移产生脏 diff。
func canonicalScope(v any) any {
	o, ok := v.(*OrderedMap)
	if !ok {
		return v
	}
	rank := func(k string) int {
		for i, sk := range scopeKeyOrder {
			if sk == k {
				return i
			}
		}
		return len(scopeKeyOrder)
	}
	keys := append([]string(nil), o.Keys...)
	sort.SliceStable(keys, func(i, j int) bool {
		ri, rj := rank(keys[i]), rank(keys[j])
		if ri != rj {
			return ri < rj
		}
		return keys[i] < keys[j]
	})
	return &OrderedMap{Keys: keys, M: o.M}
}

// leafBreakKeys 叶子节点中需换行独占的字段（缩进 +2），与仓库手写风格一致；
// error_grade 等短字段保持与 code/name 同行。
var leafBreakKeys = map[string]bool{"description": true, "device_scope": true, "tenant_tag": true}

// compactLeaf 叶子节点按原始键序输出；description / device_scope / tenant_tag 换行独占，
// 键位置不强制挪尾，与仓库手写风格一致。
func compactLeaf(o *OrderedMap, indent string) string {
	var b strings.Builder
	b.WriteString("{ ")
	for i, k := range o.Keys {
		v := o.M[k]
		if k == "device_scope" {
			v = canonicalScope(v)
		}
		brk := leafBreakKeys[k]
		if i > 0 {
			b.WriteString(",")
			if !brk {
				b.WriteString(" ")
			}
		}
		if brk {
			b.WriteString("\n" + indent + "  ")
		}
		b.WriteString(marshalKey(k) + ": " + inline(v))
	}
	b.WriteString(" }")
	return b.String()
}

func anyOrdered(a []any) bool {
	for _, e := range a {
		if _, ok := e.(*OrderedMap); ok {
			return true
		}
	}
	return false
}

func render(v any, indent string) string {
	if isLeaf(v) {
		return compactLeaf(v.(*OrderedMap), indent)
	}
	if o, ok := v.(*OrderedMap); ok {
		if len(o.Keys) == 0 {
			return "{}"
		}
		inner := indent + "  "
		lines := []string{"{"}
		for i, k := range o.Keys {
			comma := ","
			if i == len(o.Keys)-1 {
				comma = ""
			}
			val := o.M[k]
			if arr, isArr := val.([]any); isArr && anyOrdered(arr) {
				// 节点数组：多行展开，元素缩进 +4
				lines = append(lines, inner+marshalKey(k)+": [")
				elIndent := indent + "    "
				for j, el := range arr {
					c := ","
					if j == len(arr)-1 {
						c = ""
					}
					lines = append(lines, elIndent+render(el, elIndent)+c)
				}
				lines = append(lines, inner+"]"+comma)
			} else {
				lines = append(lines, inner+marshalKey(k)+": "+inline(val)+comma)
			}
		}
		lines = append(lines, indent+"}")
		return strings.Join(lines, "\n")
	}
	return inline(v)
}

// DumpCompact 序列化为仓库紧凑风格文本，末尾带换行。
func DumpCompact(data any) string {
	return render(data, "") + "\n"
}

// CleanTagData 原地清理编辑态产生的空值，避免写回仓库产生脏 diff：
// 空 device_scope、空 error_grade（""）、device_scope 内空数组、空 tenant_tag。
func CleanTagData(v any) {
	switch t := v.(type) {
	case []any:
		for _, e := range t {
			CleanTagData(e)
		}
	case *OrderedMap:
		if ds, ok := t.M["device_scope"].(*OrderedMap); ok {
			for _, k := range ds.Keys {
				if arr, isArr := ds.M[k].([]any); isArr && len(arr) == 0 {
					delete(ds.M, k)
				}
			}
			ds.Keys = filterKeys(ds.Keys, ds.M)
			if len(ds.Keys) == 0 {
				delete(t.M, "device_scope")
			}
		}
		if eg, ok := t.M["error_grade"].(string); ok && strings.TrimSpace(eg) == "" {
			delete(t.M, "error_grade")
		}
		if tt, ok := t.M["tenant_tag"].([]any); ok && len(tt) == 0 {
			delete(t.M, "tenant_tag")
		}
		t.Keys = filterKeys(t.Keys, t.M)
		for _, k := range t.Keys {
			CleanTagData(t.M[k])
		}
	}
}

// filterKeys 就地过滤掉已不在 m 中的键（读写同切片，写下标不大于读下标，安全）。
func filterKeys(keys []string, m map[string]any) []string {
	out := keys[:0]
	for _, k := range keys {
		if _, ok := m[k]; ok {
			out = append(out, k)
		}
	}
	return out
}

// normalize 转为可 DeepEqual 的普通结构（map/slice/标量）。
func normalize(v any) any {
	switch t := v.(type) {
	case *OrderedMap:
		m := map[string]any{}
		for _, k := range t.Keys {
			m[k] = normalize(t.M[k])
		}
		return m
	case []any:
		if t == nil {
			return []any{}
		}
		a := make([]any, len(t))
		for i, e := range t {
			a[i] = normalize(e)
		}
		return a
	default:
		return t
	}
}

// RoundTripOK 校验序列化结果语义与原始数据一致。
func RoundTripOK(text string, orig any) bool {
	re, err := ParseOrdered(strings.NewReader(text))
	if err != nil {
		return false
	}
	return reflect.DeepEqual(normalize(re), normalize(orig))
}
