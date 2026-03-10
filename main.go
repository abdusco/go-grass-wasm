package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"go-grass/grass"
)

type stringList []string

func (s *stringList) String() string {
	return fmt.Sprintf("%v", []string(*s))
}

func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func main() {
	in := flag.String("i", "-", "Input SCSS file path or - for stdin")
	out := flag.String("o", "-", "Output CSS file path or - for stdout")
	style := flag.String("style", "expanded", "Output style: expanded or compressed")

	includeDirs := stringList{}
	flag.Var(&includeDirs, "include-dir", "Directory to resolve imports from; repeatable")

	flag.Parse()

	parsedStyle, err := parseStyle(*style)
	if err != nil {
		fmt.Fprintf(os.Stderr, "go-grass: %v\n", err)
		os.Exit(1)
	}

	opts := grass.Options{Style: parsedStyle, IncludeDirs: includeDirs}

	ctx := context.Background()
	compiler, err := grass.NewCompiler(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "go-grass: %v\n", err)
		os.Exit(1)
	}
	defer compiler.Close(ctx)

	var css []byte
	if *in == "-" {
		src, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "go-grass: reading stdin: %v\n", err)
			os.Exit(1)
		}

		css, err = compiler.CompileString(ctx, string(src), opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "go-grass: %v\n", err)
			os.Exit(1)
		}
	} else {
		css, err = compiler.CompilePath(ctx, *in, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "go-grass: %v\n", err)
			os.Exit(1)
		}
	}

	if *out == "-" {
		if _, err := os.Stdout.Write(css); err != nil {
			fmt.Fprintf(os.Stderr, "go-grass: writing stdout: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := os.WriteFile(*out, css, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "go-grass: writing %s: %v\n", *out, err)
		os.Exit(1)
	}
}

func parseStyle(value string) (grass.Style, error) {
	switch value {
	case "expanded", "":
		return grass.StyleExpanded, nil
	case "compressed":
		return grass.StyleCompressed, nil
	default:
		return grass.StyleExpanded, fmt.Errorf("invalid --style %q (expected expanded or compressed)", value)
	}
}
