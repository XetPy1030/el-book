package wiki

import (
	"testing"

	"el-book/sdk/ui"
)

func TestBlocks(t *testing.T) {
	text := "Kobo — канадская компания.\n\n== История ==\nОснована в 2009 году.\n=== Ранние годы ===\nСначала — читалка для смартфонов.\n\n== Ссылки ==\n\n== Литература ==\n"
	got := blocks("Kobo", text)
	want := []ui.Block{
		{Kind: ui.Heading, Text: "Kobo"},
		{Kind: ui.Para, Text: "Kobo — канадская компания."},
		{Kind: ui.Heading, Text: "История"},
		{Kind: ui.Para, Text: "Основана в 2009 году."},
		{Kind: ui.Subheading, Text: "Ранние годы"},
		{Kind: ui.Para, Text: "Сначала — читалка для смартфонов."},
	}
	if len(got) != len(want) {
		t.Fatalf("абзацев %d, ждали %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("абзац %d: %+v, ждали %+v", i, got[i], want[i])
		}
	}
}
