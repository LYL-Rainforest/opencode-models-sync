package main

import (
	"fmt"
	"sort"

	"github.com/tailscale/hujson"
)

// SyncOptions 控制同步行为。
type SyncOptions struct {
	ProviderKey    string // 目标 provider 名称，例如 lmstudio
	NameSuffix     string // 新增模型显示名追加的后缀，例如 " (local)"
	Prune          bool   // 是否删除 LM Studio 中已不存在但配置仍保留的模型
	CreateProvider bool   // provider 节点不存在时是否自动创建
	BaseURL        string // 新建 provider 时写入的 baseURL
}

// SyncReport 描述本次同步产生的变更。
type SyncReport struct {
	ConfigPath       string
	Schema           string
	Added            []string
	Updated          []string
	Removed          []string
	Unchanged        []string
	CreatedContainer bool
	CreatedProvider  bool
	CreatedModels    bool
}

// Changed 报告是否产生了实际修改。
func (r *SyncReport) Changed() bool {
	return len(r.Added) > 0 || len(r.Updated) > 0 || len(r.Removed) > 0
}

// SyncModels 将 ids 同步进配置的 models 节点。
// 兼容两套 schema：新版 providers.<key>.models 与旧版 provider.<key>.models。
// 合并策略：已存在的模型条目原样保留（不覆盖用户自定义字段），仅在缺失 name 时补齐；
// 新增条目沿用现有条目的排版风格；未改动区域的字节保持原样。
func SyncModels(root *hujson.Value, ids []string, opts SyncOptions) (*SyncReport, error) {
	rootObj := objectOf(root)
	if rootObj == nil {
		return nil, fmt.Errorf("配置根节点不是 JSON 对象，无法同步")
	}

	rep := &SyncReport{}

	// ---- 1. 定位容器节点：优先 providers（新 schema），兼容 provider（旧 schema） ----
	containerKey := "providers"
	newSchema := true
	if m := findMember(rootObj, "providers"); m != nil && objectOf(&m.Value) != nil {
		rep.Schema = "providers（新 schema）"
	} else if m := findMember(rootObj, "provider"); m != nil && objectOf(&m.Value) != nil {
		containerKey = "provider"
		newSchema = false
		rep.Schema = "provider（旧 schema）"
	} else {
		if !opts.CreateProvider {
			return nil, fmt.Errorf("配置中既无 providers 也无 provider 节点；如需自动创建请加参数 -create-provider")
		}
		rep.Schema = "providers（新 schema，自动创建）"
	}

	rootIndent := memberIndentOr(rootObj, []byte("\n  "))
	containerMemberIndent := childIndent(rootIndent)

	var containerObj *hujson.Object
	createdContainer := false
	if m := findMember(rootObj, containerKey); m != nil {
		containerObj = objectOf(&m.Value)
		if containerObj == nil {
			return nil, fmt.Errorf("配置中的 %s 节点不是对象", containerKey)
		}
		containerMemberIndent = memberIndentOr(containerObj, containerMemberIndent)
	} else {
		val := newObjectValue([]byte(" "), nil)
		containerObj = val.Value.(*hujson.Object)
		appendMember(rootObj, containerKey, rootIndent, val)
		createdContainer = true
		rep.CreatedContainer = true
	}

	// ---- 2. 定位或创建 provider 节点 ----
	providerMemberIndent := childIndent(containerMemberIndent)
	var providerObj *hujson.Object
	if m := findMember(containerObj, opts.ProviderKey); m != nil {
		providerObj = objectOf(&m.Value)
		if providerObj == nil {
			return nil, fmt.Errorf("%s.%s 不是对象，无法写入 models", containerKey, opts.ProviderKey)
		}
		providerMemberIndent = memberIndentOr(providerObj, providerMemberIndent)
	} else {
		if !opts.CreateProvider {
			return nil, fmt.Errorf("未找到 %s.%s 节点；如需自动创建请加参数 -create-provider",
				containerKey, opts.ProviderKey)
		}
		val := buildProviderObject(newSchema, providerMemberIndent, opts)
		providerObj = val.Value.(*hujson.Object)
		appendMember(containerObj, opts.ProviderKey, containerMemberIndent, val)
		rep.CreatedProvider = true
	}
	if createdContainer && len(containerObj.Members) > 0 {
		containerObj.AfterExtra = closingIndent(containerMemberIndent)
	}

	// ---- 3. 定位或创建 models 节点 ----
	modelsMemberIndent := childIndent(providerMemberIndent)
	var modelsObj *hujson.Object
	createdModels := false
	if m := findMember(providerObj, "models"); m != nil {
		modelsObj = objectOf(&m.Value)
		if modelsObj == nil {
			return nil, fmt.Errorf("%s.%s.models 不是对象", containerKey, opts.ProviderKey)
		}
		modelsMemberIndent = memberIndentOr(modelsObj, modelsMemberIndent)
	} else {
		val := newObjectValue([]byte(" "), nil)
		modelsObj = val.Value.(*hujson.Object)
		appendMember(providerObj, "models", providerMemberIndent, val)
		createdModels = true
		rep.CreatedModels = true
	}
	entryMemberIndent := childIndent(modelsMemberIndent)

	// ---- 4. 合并模型条目 ----
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}

	// 以首个现有条目作为排版模板，保证新增条目风格一致
	var template *hujson.ObjectMember
	if len(modelsObj.Members) > 0 {
		template = &modelsObj.Members[0]
	}
	modelsWasEmpty := len(modelsObj.Members) == 0

	for _, id := range ids {
		display := id
		if opts.NameSuffix != "" {
			display = id + opts.NameSuffix
		}

		m := findMember(modelsObj, id)
		if m == nil {
			modelsObj.Members = append(modelsObj.Members,
				buildModelEntry(id, display, modelsMemberIndent, entryMemberIndent, template))
			rep.Added = append(rep.Added, id)
			continue
		}

		// 已存在：保留用户自定义，仅在缺 name 字段时补齐
		entryObj := objectOf(&m.Value)
		if entryObj == nil {
			rep.Unchanged = append(rep.Unchanged, id)
			continue
		}
		if findMember(entryObj, "name") != nil {
			rep.Unchanged = append(rep.Unchanged, id)
			continue
		}
		entryWasEmpty := len(entryObj.Members) == 0
		appendMember(entryObj, "name", memberIndentOr(entryObj, entryMemberIndent),
			newStringValue(display, []byte(" ")))
		if entryWasEmpty {
			entryObj.AfterExtra = closingIndent(entryMemberIndent)
		}
		rep.Updated = append(rep.Updated, id)
	}

	// ---- 5. 可选清理：删除 LM Studio 中已不存在的模型 ----
	if opts.Prune {
		for i := len(modelsObj.Members) - 1; i >= 0; i-- {
			key := memberKey(&modelsObj.Members[i])
			if wanted[key] {
				continue
			}
			modelsObj.Members = append(modelsObj.Members[:i], modelsObj.Members[i+1:]...)
			rep.Removed = append(rep.Removed, key)
		}
	}

	// ---- 6. 收尾：为新建的节点补齐右花括号缩进 ----
	if createdModels && len(modelsObj.Members) > 0 {
		modelsObj.AfterExtra = closingIndent(modelsMemberIndent)
	}
	if modelsWasEmpty && len(modelsObj.Members) > 0 && modelsObj.AfterExtra == nil {
		modelsObj.AfterExtra = closingIndent(modelsMemberIndent)
	}

	sort.Strings(rep.Added)
	sort.Strings(rep.Updated)
	sort.Strings(rep.Removed)
	sort.Strings(rep.Unchanged)
	return rep, nil
}

// buildModelEntry 构造一个模型条目。
// 若提供了模板，则深拷贝模板后替换键名与 name 值，从而完全继承现有排版（缩进、空格、注释）。
func buildModelEntry(id, display string, entryIndent, innerIndent []byte, template *hujson.ObjectMember) hujson.ObjectMember {
	if template != nil {
		m := hujson.ObjectMember{
			Name:  template.Name.Clone(),
			Value: template.Value.Clone(),
		}
		m.Name.BeforeExtra = copyExtra(entryIndent)
		m.Name.Value = hujson.String(id)
		if obj := objectOf(&m.Value); obj != nil {
			if nm := findMember(obj, "name"); nm != nil {
				nm.Value.BeforeExtra = copyExtra(nm.Value.BeforeExtra)
				nm.Value.Value = hujson.String(display)
			} else {
				appendMember(obj, "name", memberIndentOr(obj, innerIndent),
					newStringValue(display, []byte(" ")))
			}
		}
		return m
	}

	inner := &hujson.Object{
		Members: []hujson.ObjectMember{
			{
				Name:  newName("name", innerIndent),
				Value: newStringValue(display, []byte(" ")),
			},
		},
		AfterExtra: copyExtra(entryIndent),
	}
	return hujson.ObjectMember{
		Name:  newName(id, entryIndent),
		Value: hujson.Value{BeforeExtra: []byte(" "), Value: inner},
	}
}

// buildProviderObject 在 provider 缺失时构造一个可用的默认节点。
// 新 schema 使用 settings.baseURL，旧 schema 使用 options.baseURL + npm 包声明。
func buildProviderObject(newSchema bool, providerMemberIndent []byte, opts SyncOptions) hujson.Value {
	obj := &hujson.Object{}

	appendMember(obj, "name", providerMemberIndent, newStringValue("LM Studio (local)", []byte(" ")))

	if !newSchema {
		appendMember(obj, "npm", providerMemberIndent,
			newStringValue("@ai-sdk/openai-compatible", []byte(" ")))
	}

	settingsKey := "settings"
	if !newSchema {
		settingsKey = "options"
	}
	settingsIndent := childIndent(providerMemberIndent)
	settings := &hujson.Object{
		Members: []hujson.ObjectMember{
			{
				Name:  newName("baseURL", settingsIndent),
				Value: newStringValue(opts.BaseURL, []byte(" ")),
			},
		},
		AfterExtra: copyExtra(providerMemberIndent),
	}
	appendMember(obj, settingsKey, providerMemberIndent,
		hujson.Value{BeforeExtra: []byte(" "), Value: settings})

	obj.AfterExtra = closingIndent(providerMemberIndent)
	return hujson.Value{BeforeExtra: []byte(" "), Value: obj}
}
