package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const version = "1.0.0"

const usage = `api-mock v%s — run a local REST mock server from one JSON spec

USAGE
  api-mock <spec.json> [options]

OPTIONS
  -addr <host:port>   listen address (default: spec "port" or :8080)
  -validate           validate the spec and exit 0/1 (nothing is served)
  -watch-interval <d> hot-reload poll interval (default: 500ms)
  -h, --help          show this help
  -V, --version       print version

SPEC FIELDS
  port            default port when -addr is not given
  delayMs         global artificial latency for every route
  delayJitterMs   extra random 0..N ms added on top of delayMs
  routes[]        {method, path, status, headers, body, bodyFile,
                   delayMs, schema}

Paths support ":param" segments ("/users/:id"). Schemas support a small
JSON-Schema subset: type, required, properties, items, minLength,
maxLength, minimum, maximum. The spec file is watched and hot-reloaded;
a broken edit keeps the previous routes serving.`

func main() {
	log.SetFlags(0)
	var (
		addr          = flag.String("addr", "", "listen address")
		validateOnly  = flag.Bool("validate", false, "validate spec and exit")
		watchInterval = flag.Duration("watch-interval", 500*time.Millisecond, "hot-reload poll interval")
		help          = flag.Bool("h", false, "show help")
		longHelp      = flag.Bool("help", false, "show help")
		showVersion   = flag.Bool("V", false, "print version")
		longVersion   = flag.Bool("version", false, "print version")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, usage+"\n", version)
	}
	// Users write "api-mock spec.json -validate": reorder so the flag
	// package sees flags before positionals.
	flag.CommandLine.Parse(flagsFirst(os.Args[1:]))
	if *help || *longHelp {
		fmt.Printf(usage+"\n", version)
		return
	}
	if *showVersion || *longVersion {
		fmt.Println("api-mock", version)
		return
	}
	if flag.NArg() != 1 {
		failUsage("expected exactly one spec file")
	}
	specPath := flag.Arg(0)

	server, err := NewServer(specPath)
	if err != nil {
		failRuntime(err)
	}
	if *validateOnly {
		fmt.Printf("spec ok: %d routes\n", len(server.current().Routes))
		return
	}

	listen := *addr
	if listen == "" {
		if port := server.current().Port; port != 0 {
			listen = fmt.Sprintf(":%d", port)
		} else {
			listen = ":8080"
		}
	}

	go server.Watch(*watchInterval, log.New(os.Stderr, "api-mock: ", log.LstdFlags))

	log.Printf("api-mock %s serving %d routes from %s on http://localhost%s", version, len(server.current().Routes), specPath, listen)
	if err := http.ListenAndServe(listen, server); err != nil {
		failRuntime(err)
	}
}

func failUsage(format string, args ...any) {
	log.Printf(format, args...)
	os.Exit(2)
}

func failRuntime(err error) {
	log.Printf("api-mock: %v", err)
	os.Exit(1)
}

// flagsFirst moves flag tokens (and their values) before positional
// arguments, accepting "api-mock spec.json -validate" as well as the
// canonical "api-mock -validate spec.json".
func flagsFirst(args []string) []string {
	takesValue := map[string]bool{
		"-addr": true, "--addr": true,
		"-watch-interval": true, "--watch-interval": true,
	}
	var flags, rest []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") && arg != "-" && arg != "--" {
			flags = append(flags, arg)
			if takesValue[arg] && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		rest = append(rest, arg)
	}
	return append(flags, rest...)
}
