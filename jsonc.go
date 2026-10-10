package main

import "github.com/tailscale/hujson"

// objectOf 返回 v 底层的 *hujson.Object；若 v 不是对象则返回 nil。
func objectOf(v *hujson.Value) *hujson.Object {
	if v == nil {
		return nil
	}
	obj, _ := v.Value.(*hujson.Object)
	return obj
}

// memberKey 返回成员键名（自动去掉 JSON 字符串引号与转义）。
func memberKey(m *hujson.ObjectMember) string {
	if l, ok := m.Name.Value.(hujson.Literal); ok {
		return l.String()
	}
	return ""
}

// findMember 按键名查找成员，不存在返回 nil。
func findMember(obj *hujson.Object, key string) *hujson.ObjectMember {
	if obj == nil {
		return nil
	}
	for i := range obj.Members {
		if memberKey(&obj.Members[i]) == key {
			return &obj.Members[i]
		}
	}
	return nil
}

// copyExtra 复制 extra 字节，避免多个节点共享同一底层数组。
func copyExtra(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

// childIndent 由父级成员缩进推导下一级缩进（追加两个空格）。
func childIndent(indent []byte) []byte {
	out := make([]byte, len(indent), len(indent)+2)
	copy(out, indent)
	return append(out, ' ', ' ')
}

// closingIndent 推导对象右花括号的缩进（比成员缩进回退一级）。
func closingIndent(indent []byte) []byte {
	if len(indent) >= 2 {
		return copyExtra(indent[:len(indent)-2])
	}
	return copyExtra(indent)
}

// memberIndent 返回该对象成员实际使用的缩进（取自首个成员）；无成员返回 nil。
func memberIndent(obj *hujson.Object) []byte {
	if obj == nil || len(obj.Members) == 0 {
		return nil
	}
	return copyExtra(obj.Members[0].Name.BeforeExtra)
}

// memberIndentOr 优先取对象现有缩进，否则用给定回退值。
func memberIndentOr(obj *hujson.Object, fallback []byte) []byte {
	if ind := memberIndent(obj); ind != nil {
		return ind
	}
	return copyExtra(fallback)
}

// newName 构造成员键节点。
func newName(key string, indent []byte) hujson.Value {
	return hujson.Value{BeforeExtra: copyExtra(indent), Value: hujson.String(key)}
}

// newStringValue 构造 JSON 字符串值节点，before 为冒号后的空白（通常为单个空格）。
func newStringValue(s string, before []byte) hujson.Value {
	return hujson.Value{BeforeExtra: copyExtra(before), Value: hujson.String(s)}
}

// newObjectValue 构造空对象节点。
func newObjectValue(before, after []byte) hujson.Value {
	return hujson.Value{
		BeforeExtra: copyExtra(before),
		Value:       &hujson.Object{AfterExtra: copyExtra(after)},
	}
}

// appendMember 向对象追加成员；值若未指定前置空白，默认补一个空格（`"key": value`）。
func appendMember(obj *hujson.Object, key string, indent []byte, val hujson.Value) *hujson.ObjectMember {
	if val.BeforeExtra == nil {
		val.BeforeExtra = []byte(" ")
	}
	obj.Members = append(obj.Members, hujson.ObjectMember{
		Name:  newName(key, indent),
		Value: val,
	})
	return &obj.Members[len(obj.Members)-1]
}

// removeMember 删除指定键，返回是否命中。
func removeMember(obj *hujson.Object, key string) bool {
	for i := range obj.Members {
		if memberKey(&obj.Members[i]) == key {
			obj.Members = append(obj.Members[:i], obj.Members[i+1:]...)
			return true
		}
	}
	return false
}
