// Command testreport runs `go test -json` and writes the results as a
// Markdown report. Access tokens in the test output are masked.
//
//	go run ./cmd/testreport -out reports/unit.md -cover
//	go run ./cmd/testreport -tags integration -run Integration -out reports/integration.md
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

type event struct {
	Action  string
	Package string
	Test    string
	Elapsed float64
	Output  string
}

type result struct {
	Name    string
	Status  string // pass, fail, skip, running
	Elapsed float64
	Output  []string
	order   int
}

var tokenRe = regexp.MustCompile(`\b(TEST|APP_USR)-[0-9a-fA-F][\w-]{6,}`)

func mask(s string) string { return tokenRe.ReplaceAllString(s, "$1-***") }

func main() {
	var (
		tags  = flag.String("tags", "", "build tags passed to go test")
		run   = flag.String("run", "", "-run pattern passed to go test")
		out   = flag.String("out", "reports/test-report.md", "output Markdown file")
		title = flag.String("title", "", "report title")
		cover = flag.Bool("cover", false, "include a coverage section")
		pkg   = flag.String("pkg", ".", "package to test")
	)
	flag.Parse()

	if *title == "" {
		*title = "Relatório de testes"
		if *tags != "" {
			*title += " (" + *tags + ")"
		}
	}

	args := []string{"test", "-json", "-count=1"}
	if *tags != "" {
		args = append(args, "-tags", *tags)
	}
	if *run != "" {
		args = append(args, "-run", *run)
	}
	var coverFile string
	if *cover {
		f, err := os.CreateTemp("", "cover-*.out")
		must(err)
		f.Close()
		coverFile = f.Name()
		defer os.Remove(coverFile)
		args = append(args, "-coverprofile", coverFile)
	}
	args = append(args, *pkg)

	start := time.Now()
	cmd := exec.Command("go", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	must(err)
	must(cmd.Start())
	results, pkgOutput := parse(stdout)
	runErr := cmd.Wait()
	wall := time.Since(start)

	var coverage string
	if coverFile != "" && runErr == nil {
		if b, err := exec.Command("go", "tool", "cover", "-func", coverFile).Output(); err == nil {
			coverage = string(b)
		}
	}

	report := render(*title, "go "+strings.Join(args, " "), results, pkgOutput, stderr.String(), coverage, wall)
	must(os.MkdirAll(filepath.Dir(*out), 0o755))
	must(os.WriteFile(*out, []byte(report), 0o644))

	pass, fail, skip := count(results)
	fmt.Printf("%s: %d passed, %d failed, %d skipped -> %s\n", *title, pass, fail, skip, *out)
	if runErr != nil || fail > 0 {
		os.Exit(1)
	}
}

func parse(r io.Reader) (map[string]*result, []string) {
	results := map[string]*result{}
	var pkgOutput []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var e event
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			continue
		}
		if e.Test == "" {
			if e.Action == "output" {
				pkgOutput = append(pkgOutput, mask(strings.TrimRight(e.Output, "\n")))
			}
			continue
		}
		res, ok := results[e.Test]
		if !ok {
			res = &result{Name: e.Test, Status: "running", order: len(results)}
			results[e.Test] = res
		}
		switch e.Action {
		case "pass", "fail", "skip":
			res.Status, res.Elapsed = e.Action, e.Elapsed
		case "output":
			line := strings.TrimRight(e.Output, "\n")
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "=== ") || strings.HasPrefix(trimmed, "--- ") || trimmed == "" {
				continue
			}
			res.Output = append(res.Output, mask(trimmed))
		}
	}
	return results, pkgOutput
}

func count(results map[string]*result) (pass, fail, skip int) {
	for _, r := range results {
		switch r.Status {
		case "pass":
			pass++
		case "skip":
			skip++
		default:
			fail++
		}
	}
	return
}

var icons = map[string]string{"pass": "✅ passou", "fail": "❌ falhou", "skip": "⏭️ pulado", "running": "💥 interrompido"}

func render(title, command string, results map[string]*result, pkgOutput []string, stderr, coverage string, wall time.Duration) string {
	list := make([]*result, 0, len(results))
	for _, r := range results {
		list = append(list, r)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].order < list[j].order })
	pass, fail, skip := count(results)

	overall := "✅ **Todos os testes passaram**"
	if fail > 0 || len(results) == 0 {
		overall = "❌ **Há falhas**"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "%s\n\n", overall)
	fmt.Fprintf(&b, "- **Data:** %s\n", time.Now().Format("02/01/2006 15:04:05 MST"))
	fmt.Fprintf(&b, "- **Go:** %s (%s/%s)\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "- **Comando:** `%s`\n\n", command)

	b.WriteString("## Resumo\n\n| Total | Passou | Falhou | Pulado | Duração |\n|---:|---:|---:|---:|---:|\n")
	fmt.Fprintf(&b, "| %d | %d | %d | %d | %s |\n\n", len(list), pass, fail, skip, wall.Round(10*time.Millisecond))

	b.WriteString("## Resultados\n\n| # | Teste | Status | Duração |\n|---:|---|---|---:|\n")
	for i, r := range list {
		fmt.Fprintf(&b, "| %d | `%s` | %s | %.2fs |\n", i+1, r.Name, icons[r.Status], r.Elapsed)
	}
	b.WriteString("\n")

	var withLogs []*result
	for _, r := range list {
		if len(r.Output) > 0 {
			withLogs = append(withLogs, r)
		}
	}
	if len(withLogs) > 0 {
		b.WriteString("## Detalhes\n\n")
		for _, r := range withLogs {
			fmt.Fprintf(&b, "### %s — %s\n\n```text\n%s\n```\n\n", r.Name, icons[r.Status], strings.Join(r.Output, "\n"))
		}
	}

	if fail > 0 || len(results) == 0 {
		var extra []string
		for _, l := range pkgOutput {
			if l != "" && l != "PASS" && l != "FAIL" && !strings.HasPrefix(l, "ok ") {
				extra = append(extra, l)
			}
		}
		if s := strings.TrimSpace(mask(stderr)); s != "" {
			extra = append(extra, s)
		}
		if len(extra) > 0 {
			fmt.Fprintf(&b, "## Saída do pacote\n\n```text\n%s\n```\n\n", strings.Join(extra, "\n"))
		}
	}

	if coverage != "" {
		b.WriteString("## Cobertura\n\n")
		var rows []string
		total := ""
		for _, l := range strings.Split(strings.TrimSpace(coverage), "\n") {
			f := strings.Fields(l)
			if len(f) < 3 {
				continue
			}
			if f[0] == "total:" {
				total = f[len(f)-1]
				continue
			}
			file := f[0]
			if i := strings.LastIndex(file, "/"); i >= 0 {
				file = file[i+1:]
			}
			rows = append(rows, fmt.Sprintf("| `%s` | `%s` | %s |", strings.TrimSuffix(file, ":"), f[1], f[len(f)-1]))
		}
		fmt.Fprintf(&b, "**Cobertura total: %s**\n\n<details><summary>Por função</summary>\n\n| Arquivo | Função | Cobertura |\n|---|---|---:|\n%s\n\n</details>\n", total, strings.Join(rows, "\n"))
	}
	return b.String()
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "testreport:", err)
		os.Exit(2)
	}
}
