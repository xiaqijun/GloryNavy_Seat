package eve

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestLPReferenceOfficialFieldsAndMissingRecipe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sde.zip")
	f, _ := os.Create(path)
	z := zip.NewWriter(f)
	for name, body := range map[string]string{
		"npcCorporations.jsonl": "{\"_key\":1000130,\"name\":{\"zh\":\"姐妹会\",\"en\":\"Sisters of EVE\"},\"factionID\":500016}\n{\"_key\":1000001,\"deleted\":true}",
		"factions.jsonl":        `{"_key":500016,"name":{"zh":"姐妹会","en":"Servant Sisters of EVE"}}`,
		"blueprints.jsonl":      "{\"_key\":10,\"activities\":{\"manufacturing\":{\"products\":[{\"typeID\":20,\"quantity\":2}],\"materials\":[{\"typeID\":30,\"quantity\":4}]}}}\n{\"_key\":11,\"activities\":{}}",
	} {
		w, _ := z.Create(name)
		w.Write([]byte(body))
	}
	z.Close()
	f.Close()
	v, e := readLPReference(path, 123)
	if e != nil || v.Build != 123 || len(v.Corporations) != 1 || v.Corporations[0].FactionID != 500016 || v.Blueprints[10].Product.TypeID != 20 || v.Blueprints[10].Materials[0].Quantity != 4 || v.Blueprints[11].Product.TypeID != 0 {
		t.Fatal(v, e)
	}
	bad, _ := os.Create(filepath.Join(t.TempDir(), "empty.zip"))
	writer := zip.NewWriter(bad)
	writer.Close()
	bad.Close()
	if _, e := readLPReference(bad.Name(), 1); e == nil {
		t.Fatal("missing datasets accepted")
	}
}
