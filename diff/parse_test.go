package diff

import (
	"testing"
)

const sampleDiff = `diff --git a/main.go b/main.go
index 1234567..abcdefg 100644
--- a/main.go
+++ b/main.go
@@ -10,6 +10,8 @@ func main() {
 	fmt.Println("hello")
 	x := 1
-	y := 2
+	y := 3
+	z := 4
 	fmt.Println(x)
 }
`

func TestParseSingleFile(t *testing.T) {
	files := Parse(sampleDiff)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	f := files[0]
	if f.Path != "main.go" {
		t.Errorf("expected path main.go, got %s", f.Path)
	}
	if len(f.Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(f.Hunks))
	}

	h := f.Hunks[0]
	if h.OldStart != 10 || h.OldCount != 6 || h.NewStart != 10 || h.NewCount != 8 {
		t.Errorf("unexpected hunk header: old=%d,%d new=%d,%d", h.OldStart, h.OldCount, h.NewStart, h.NewCount)
	}

	if f.Additions != 2 {
		t.Errorf("expected 2 additions, got %d", f.Additions)
	}
	if f.Deletions != 1 {
		t.Errorf("expected 1 deletion, got %d", f.Deletions)
	}

	// Check line types
	expected := []LineType{LineContext, LineContext, LineDelete, LineAdd, LineAdd, LineContext, LineContext}
	if len(h.Lines) != len(expected) {
		t.Fatalf("expected %d lines, got %d", len(expected), len(h.Lines))
	}
	for i, lt := range expected {
		if h.Lines[i].Type != lt {
			t.Errorf("line %d: expected type %d, got %d", i, lt, h.Lines[i].Type)
		}
	}
}

const newFileDiff = `diff --git a/new.txt b/new.txt
new file mode 100644
index 0000000..1234567
--- /dev/null
+++ b/new.txt
@@ -0,0 +1,3 @@
+line one
+line two
+line three
`

func TestParseNewFile(t *testing.T) {
	files := Parse(newFileDiff)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	f := files[0]
	if f.Path != "new.txt" {
		t.Errorf("expected path new.txt, got %s", f.Path)
	}
	if f.Additions != 3 {
		t.Errorf("expected 3 additions, got %d", f.Additions)
	}
	if f.Deletions != 0 {
		t.Errorf("expected 0 deletions, got %d", f.Deletions)
	}
}

func TestParseMultipleFiles(t *testing.T) {
	multi := sampleDiff + newFileDiff
	files := Parse(multi)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if files[0].Path != "main.go" {
		t.Errorf("expected first file main.go, got %s", files[0].Path)
	}
	if files[1].Path != "new.txt" {
		t.Errorf("expected second file new.txt, got %s", files[1].Path)
	}
}

func TestParseLineNumbers(t *testing.T) {
	files := Parse(sampleDiff)
	h := files[0].Hunks[0]

	// First context line should be old=10, new=10
	if h.Lines[0].OldNum != 10 || h.Lines[0].NewNum != 10 {
		t.Errorf("line 0: expected 10/10, got %d/%d", h.Lines[0].OldNum, h.Lines[0].NewNum)
	}

	// Deletion (y := 2) should be old=12, new=0
	if h.Lines[2].OldNum != 12 || h.Lines[2].NewNum != 0 {
		t.Errorf("deletion: expected old=12/new=0, got %d/%d", h.Lines[2].OldNum, h.Lines[2].NewNum)
	}

	// First addition (y := 3) should be old=0, new=12
	if h.Lines[3].OldNum != 0 || h.Lines[3].NewNum != 12 {
		t.Errorf("addition: expected old=0/new=12, got %d/%d", h.Lines[3].OldNum, h.Lines[3].NewNum)
	}
}
