package storage

import fsys "os"

func save() { _ = fsys.WriteFile("a", nil, 0o644) }
