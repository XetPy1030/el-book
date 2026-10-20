package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Source отдаёт файлы репозитория из одного состояния: коммита, индекса или рабочего дерева.
// Пути — от корня репозитория, через «/».
type Source interface {
	Name() string
	Files() ([]string, error)
	Read(path string) ([]byte, error)
}

// NewSource выбирает источник по значению флага -src.
func NewSource(kind string) (Source, error) {
	switch kind {
	case "head":
		if err := exec.Command("git", "rev-parse", "--verify", "--quiet", "HEAD").Run(); err != nil {
			return nil, errors.New("в репозитории ещё нет коммитов: запустите с -src=index или -src=worktree")
		}
		return gitSource{name: "HEAD", rev: "HEAD"}, nil
	case "index":
		return gitSource{name: "индекс", rev: ""}, nil
	case "worktree":
		return worktreeSource{}, nil
	}
	return nil, fmt.Errorf("неизвестный источник %q: нужен head, index или worktree", kind)
}

// gitSource читает из коммита (rev = "HEAD") или из индекса (rev = "").
type gitSource struct {
	name string
	rev  string
}

func (s gitSource) Name() string { return s.name }

func (s gitSource) Files() ([]string, error) {
	args := []string{"ls-files", "-z"}
	if s.rev != "" {
		args = []string{"ls-tree", "-r", "--name-only", "-z", s.rev}
	}
	out, err := git(args...)
	if err != nil {
		return nil, err
	}
	return splitZ(out), nil
}

func (s gitSource) Read(path string) ([]byte, error) {
	return git("show", s.rev+":"+path)
}

// worktreeSource читает файлы с диска, но список берёт у git: игнорируемое (backup/) не видно.
type worktreeSource struct{}

func (worktreeSource) Name() string { return "рабочее дерево" }

func (worktreeSource) Files() ([]string, error) {
	out, err := git("ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, f := range splitZ(out) {
		if _, err := os.Stat(f); err == nil {
			files = append(files, f)
		}
	}
	return files, nil
}

func (worktreeSource) Read(path string) ([]byte, error) { return os.ReadFile(path) }

func git(args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func splitZ(b []byte) []string {
	var out []string
	for _, p := range bytes.Split(b, []byte{0}) {
		if len(p) > 0 {
			out = append(out, string(p))
		}
	}
	return out
}
