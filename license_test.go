// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every hand-written .go file in this module carries the SPDX header.
// generated/ is excluded — oapi-codegen owns those files and would overwrite
// a hand-added header on the next regenerate.
func TestLicenseHeaders(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	var files []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == "generated" || base == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("expected to find at least one .go file")
	}

	var missing []string
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		head := string(data)
		if len(head) > 200 {
			head = head[:200]
		}
		if !strings.Contains(head, "SPDX-License-Identifier: MIT") ||
			!strings.Contains(head, "Copyright (c) 2026 Truestock") {
			missing = append(missing, file)
		}
	}

	if len(missing) > 0 {
		t.Fatalf("missing licence header:\n%s", strings.Join(missing, "\n"))
	}
}

// The LICENSE file itself is MIT and names the copyright holder.
func TestLicenseFile(t *testing.T) {
	data, err := os.ReadFile("LICENSE")
	if err != nil {
		t.Fatal(err)
	}
	licence := string(data)
	for _, want := range []string{"MIT License", "Copyright (c) 2026 Truestock", "WITHOUT WARRANTY OF ANY KIND"} {
		if !strings.Contains(licence, want) {
			t.Errorf("LICENSE missing %q", want)
		}
	}
}
