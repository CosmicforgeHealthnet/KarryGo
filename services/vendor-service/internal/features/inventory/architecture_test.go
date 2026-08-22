package inventory

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestSection14AInventoryDoesNotImportShop(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)

	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	dir := filepath.Dir(file)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	set := token.NewFileSet()

	for _, entry := range entries {
		if entry.IsDir() ||
			!strings.HasSuffix(entry.Name(), ".go") {
			continue
		}

		parsed, err := parser.ParseFile(
			set,
			filepath.Join(dir, entry.Name()),
			nil,
			parser.ImportsOnly,
		)
		if err != nil {
			t.Fatalf(
				"ParseFile(%s): %v",
				entry.Name(),
				err,
			)
		}

		for _, spec := range parsed.Imports {
			path, err := strconv.Unquote(
				spec.Path.Value,
			)
			if err != nil {
				t.Fatalf(
					"Unquote import: %v",
					err,
				)
			}

			if strings.Contains(
				path,
				"/internal/features/shop",
			) {
				t.Fatalf(
					"%s imports Shop implementation %q",
					entry.Name(),
					path,
				)
			}
		}
	}
}
