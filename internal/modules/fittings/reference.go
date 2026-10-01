package fittings

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"glorynavy.local/seat/internal/modules/eve"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

//go:embed reference.json
var referenceJSON []byte

type RequiredSkill struct {
	ID    int64 `json:"skill_id,string"`
	Level int   `json:"level"`
}
type referenceType struct {
	ID           int64           `json:"id,string"`
	Name         string          `json:"name"`
	English      string          `json:"english"`
	Category     int             `json:"category"`
	Group        string          `json:"group"`
	GroupEnglish string          `json:"group_english"`
	Slot         string          `json:"slot"`
	Requirements []RequiredSkill `json:"requirements"`
}

var referenceBuild int64
var references = func() map[int64]referenceType {
	var v struct {
		Build int64           `json:"build"`
		Types []referenceType `json:"types"`
	}
	if err := json.Unmarshal(referenceJSON, &v); err != nil {
		panic(err)
	}
	referenceBuild = v.Build
	out := map[int64]referenceType{}
	for _, t := range v.Types {
		out[t.ID] = t
	}
	return out
}()
var referenceNames = func() map[string]int64 {
	out := map[string]int64{}
	for id, t := range references {
		out[strings.ToLower(t.Name)] = id
		out[strings.ToLower(t.English)] = id
	}
	return out
}()

type ImportError struct{ Message string }

func (e ImportError) Error() string      { return e.Message }
func invalidImport(message string) error { return ImportError{message} }

var eftStack = regexp.MustCompile(`^(.*?)\s+x([0-9]+)$`)
var eftEmpty = regexp.MustCompile(`(?i)^\[Empty (low|med|high|rig|service) slot\]$`)

func parseEFT(text string) (Fit, error) {
	f := Fit{SkillMode: "all5", Items: []Item{}}
	if len(text) > 200000 {
		return f, invalidImport("配装文本过长")
	}
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(text), "\r", ""), "\n")
	header := strings.TrimSpace(lines[0])
	if !strings.HasPrefix(header, "[") || !strings.HasSuffix(header, "]") {
		return f, invalidImport("首行应为 [舰船名称, 方案名称]")
	}
	ship, name, ok := strings.Cut(header[1:len(header)-1], ",")
	if !ok {
		return f, invalidImport("请填写舰船和方案名称")
	}
	f.Name = strings.TrimSpace(name)
	f.ShipTypeID = referenceNames[strings.ToLower(strings.TrimSpace(ship))]
	if references[f.ShipTypeID].Category != 6 {
		return f, invalidImport("未识别舰船名称")
	}
	indices := map[string]int{}
	stackIndex := 0
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if m := eftEmpty.FindStringSubmatch(line); m != nil {
			slot := strings.ToLower(m[1])
			if slot == "med" {
				slot = "medium"
			}
			indices[slot]++
			continue
		}
		offline := strings.HasSuffix(strings.ToLower(line), "/offline")
		if offline {
			line = strings.TrimSpace(line[:len(line)-len("/offline")])
		}
		quantity := int64(1)
		stack := false
		if m := eftStack.FindStringSubmatch(line); m != nil {
			stack = true
			var err error
			quantity, err = strconv.ParseInt(m[2], 10, 64)
			if err != nil || quantity < 1 || quantity > 1000000 {
				return f, invalidImport("物品数量无效")
			}
			line = m[1]
		}
		parts := strings.Split(line, ",")
		if len(parts) > 2 {
			return f, invalidImport("无法识别物品行：" + line)
		}
		id := referenceNames[strings.ToLower(strings.TrimSpace(parts[0]))]
		t, ok := references[id]
		if !ok {
			return f, invalidImport("未识别物品：" + strings.TrimSpace(parts[0]))
		}
		slot := t.Slot
		if slot == "" {
			slot = "cargo"
		}
		if stack && slot != "drone_bay" && slot != "fighter_bay" {
			slot = "cargo"
		}
		index := indices[slot]
		if slices.Contains([]string{"cargo", "drone_bay", "fighter_bay"}, slot) {
			index = stackIndex
			stackIndex++
		} else {
			indices[slot]++
			if quantity != 1 {
				return f, invalidImport("已装配装备须逐行填写")
			}
		}
		var charge int64
		if len(parts) == 2 {
			charge = referenceNames[strings.ToLower(strings.TrimSpace(parts[1]))]
			if references[charge].Category != 8 || slot == "cargo" {
				return f, invalidImport("无法识别装填弹药：" + parts[1])
			}
		}
		state := "active"
		if offline {
			state = "offline"
		}
		f.Items = append(f.Items, Item{TypeID: id, Slot: slot, Index: index, Quantity: quantity, State: state, ChargeID: charge})
	}
	if err := validate(f); err != nil {
		return f, invalidImport("请检查方案名称、槽位数量和物品")
	}
	if len([]rune(f.Name)) > 50 {
		return f, invalidImport("游戏方案名称最多 50 个字符")
	}
	return f, nil
}
func prerequisites(f Fit) ([]RequiredSkill, error) {
	required := map[int64]int{}
	visited := map[int64]bool{}
	visiting := map[int64]bool{}
	var walk func(int64) error
	walk = func(id int64) error {
		if visited[id] {
			return nil
		}
		if visiting[id] {
			return invalidImport("静态技能依赖存在循环")
		}
		t, ok := references[id]
		if !ok {
			return invalidImport(fmt.Sprintf("静态数据缺少物品 #%d", id))
		}
		visiting[id] = true
		for _, r := range t.Requirements {
			if r.Level < 1 || r.Level > 5 || references[r.ID].Category != 16 {
				return invalidImport("静态技能要求不完整")
			}
			if r.Level > required[r.ID] {
				required[r.ID] = r.Level
			}
			if err := walk(r.ID); err != nil {
				return err
			}
		}
		visiting[id] = false
		visited[id] = true
		return nil
	}
	if err := walk(f.ShipTypeID); err != nil {
		return nil, err
	}
	for _, i := range f.Items {
		if i.Slot != "cargo" || references[i.TypeID].Category == 8 {
			if err := walk(i.TypeID); err != nil {
				return nil, err
			}
		}
		if i.ChargeID > 0 {
			if err := walk(i.ChargeID); err != nil {
				return nil, err
			}
		}
	}
	out := []RequiredSkill{}
	for id, level := range required {
		out = append(out, RequiredSkill{id, level})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func gamePayload(f Fit, description string) (eve.GameFitting, error) {
	out := eve.GameFitting{Name: f.Name, Description: description, ShipTypeID: f.ShipTypeID, Items: []eve.SavedFittingItem{}}
	if len([]rune(f.Name)) > 50 || len([]rune(description)) > 500 || references[f.ShipTypeID].Category != 6 {
		return out, ErrInvalid
	}
	prefixes := map[string]string{"high": "HiSlot", "medium": "MedSlot", "low": "LoSlot", "rig": "RigSlot", "subsystem": "SubSystemSlot", "service": "ServiceSlot", "cargo": "Cargo", "drone_bay": "DroneBay", "fighter_bay": "FighterBay"}
	cargo := map[int64]int64{}
	for _, i := range f.Items {
		if i.Slot == "cargo" {
			cargo[i.TypeID] += i.Quantity
		}
	}
	for _, i := range f.Items {
		prefix, ok := prefixes[i.Slot]
		if !ok {
			return out, invalidImport("该槽位不支持保存到游戏")
		}
		flag := prefix
		if !slices.Contains([]string{"cargo", "drone_bay", "fighter_bay"}, i.Slot) {
			flag += strconv.Itoa(i.Index)
		}
		if i.ChargeID > 0 && cargo[i.ChargeID] == 0 {
			return out, invalidImport("请在 EFT 货舱中明确填写弹药数量：" + references[i.ChargeID].English + " x数量")
		}
		out.Items = append(out.Items, eve.SavedFittingItem{Flag: flag, TypeID: i.TypeID, Quantity: i.Quantity})
	}
	if len(out.Items) == 0 || len(out.Items) > 512 {
		return out, invalidImport("游戏方案需包含 1–512 项物品")
	}
	return out, nil
}

func exportEFT(f Fit) string {
	name := func(id int64) string {
		if t, ok := references[id]; ok {
			return t.English
		}
		return fmt.Sprintf("Type #%d", id)
	}
	lines := []string{"[" + name(f.ShipTypeID) + ", " + f.Name + "]"}
	empty := map[string]string{"low": "low", "medium": "med", "high": "high", "rig": "rig", "service": "service"}
	for _, slot := range []string{"low", "medium", "high", "rig", "subsystem", "service", "drone_bay", "fighter_bay", "cargo"} {
		items := []Item{}
		for _, i := range f.Items {
			if i.Slot == slot {
				items = append(items, i)
			}
		}
		sort.SliceStable(items, func(i, j int) bool { return items[i].Index < items[j].Index })
		at := 0
		for _, i := range items {
			if token := empty[slot]; token != "" {
				for at < i.Index {
					lines = append(lines, "[Empty "+token+" slot]")
					at++
				}
			}
			line := name(i.TypeID)
			if slices.Contains([]string{"cargo", "drone_bay", "fighter_bay"}, slot) {
				line += fmt.Sprintf(" x%d", i.Quantity)
			} else {
				if i.ChargeID > 0 {
					line += ", " + name(i.ChargeID)
				}
				if i.State == "offline" {
					line += " /offline"
				}
			}
			lines = append(lines, line)
			at++
		}
		lines = append(lines, "")
	}
	return strings.TrimSpace(strings.Join(lines, "\n")) + "\n"
}
