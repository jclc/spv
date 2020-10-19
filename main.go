package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
)

type e struct{} // empty type

const (
	genExtension     = ".gen.go"
	manifestFilename = "shaders" + genExtension
)

var (
	dir     string
	pkg     string
	verbose bool
	cc      string
	ccArgs  string
	force   bool // true if all source files should always be generated

	filesToGenerate []string
	filesToDelete   []string
	filesTotal      []string
	manifestFound   bool

	tempDir string

	validExtensions = map[string]e{
		".vert":  e{},
		".tesc":  e{},
		".tese":  e{},
		".geom":  e{},
		".frag":  e{},
		".comp":  e{},
		".mesh":  e{},
		".task":  e{},
		".rgen":  e{},
		".rint":  e{},
		".rahit": e{},
		".rchit": e{},
		".rmiss": e{},
		".rcall": e{},
	}
)

func main() {
	os.Exit(run())
}

func run() (exitcode int) {
	parseArgs()
	if dir != "" {
		err := os.Chdir(dir)
		if err != nil {
			fmt.Println("Invalid directory", dir)
			return 1
		}
	}

	if pkg == "" {
		fmt.Println("No package name specified")
		return 1
	}

	// Populates filesToGenerate, filesToDelete and manifestFound
	if c := getFiles(); c != 0 {
		return c
	}

	if len(filesToGenerate)+len(filesToDelete) == 0 && manifestFound {
		if verbose {
			fmt.Printf("%s: No changes\n", os.Args[0])
		}
		return 0
	}

	if _, err := exec.LookPath(cc); err != nil {
		fmt.Printf("%s error: Cannot find GLSL compiler %s\n", os.Args[0], cc)
		return 1
	}

	td, err := ioutil.TempDir("", "go-spv-*")
	if err != nil {
		fmt.Printf("%s error: Cannot create temp directory: %v\n", os.Args[0], err)
		return 1
	}
	tempDir = td
	defer os.RemoveAll(tempDir)

	statusChan := make(chan string)
	statusChanClosed := make(chan e)
	go func() {
		defer close(statusChanClosed)
		for s := range statusChan {
			fmt.Println(s)
		}
	}()

	var numErr uint32
	var changed uint32 // stays at 0 if none of the files were changed

	wg := sync.WaitGroup{}
	wg.Add(len(filesToGenerate))
	for _, f := range filesToGenerate {
		f := f
		go func() {
			chng, err := operate(f, statusChan)
			if err != nil {
				atomic.AddUint32(&numErr, 1)
				statusChan <- fmt.Sprintf("%s error in file %s: %v", os.Args[0], f, err)
			}

			if chng {
				atomic.StoreUint32(&changed, 1)
			}
			wg.Done()
		}()
	}
	wg.Wait()
	close(statusChan)
	<-statusChanClosed

	if numErr > 0 {
		fmt.Printf("%s: errors in %d files\n", os.Args[0], numErr)
		return 1
	}

	for _, file := range filesToDelete {
		os.Remove(file)
	}

	if changed == 1 || !manifestFound || len(filesToDelete) != 0 {
		return writeManifest()
	}

	return 0
}

func parseArgs() {
	flag.StringVar(&dir, "dir", "", "Path to the directory with the source files")
	flag.StringVar(&pkg, "pkg", "", "Package name for the output files")
	flag.BoolVar(&verbose, "verbose", false, "Enable for informative messages")
	flag.StringVar(&cc, "cc", "", "GLSL compiler")
	flag.StringVar(&ccArgs, "args", "", "GLSL compiler arguments")
	flag.BoolVar(&force, "force", false, "Force compilation for every file regardless of date modified")
	flag.Parse()

	if cc == "" {
		if runtime.GOOS == "windows" {
			cc = "glslangValidator.exe"
		} else {
			cc = "glslangValidator"
		}
	}
}

func getFiles() (exitcode int) {
	d, err := os.Stat(".")
	if os.IsNotExist(err) {
		fmt.Printf("%s error: Directory %s does not exist\n", os.Args[0], dir)
		return 1
	}

	if !d.IsDir() {
		fmt.Printf("%s error: %s is not a directory\n", os.Args[0], dir)
		return 1
	}

	fs, err := ioutil.ReadDir(".")
	if err != nil {
		fmt.Printf("%s error: Cannot read directory contents: %v\n", os.Args[0], err)
	}

	// sources is all GLSL files
	// generated are all .go files generated from GLSL files
	sources := make(map[string]e)
	generated := make(map[string]e)

	for _, f := range fs {
		if f.IsDir() {
			continue
		}

		filename := f.Name()
		switch {
		case filename == manifestFilename:
			manifestFound = true
		case isGLSLFile(filename):
			sources[filename] = e{}
		case isGeneratedFromGLSL(filename):
			generated[filename] = e{}
		}
	}

	for src := range sources {
		gen := generatedName(src)
		_, found := generated[gen]
		if force || !found || isNewer(src, gen) {
			filesToGenerate = append(filesToGenerate, src)
		}
	}

	for gen := range generated {
		if _, found := sources[originalName(gen)]; !found {
			filesToDelete = append(filesToDelete, gen)
		}
	}

	for file := range sources {
		filesTotal = append(filesTotal, file)
	}

	sort.Strings(filesTotal)

	return
}

func isGLSLFile(filename string) bool {
	ext := filepath.Ext(filename)
	if ext == ".glsl" {
		ext = filepath.Ext(filename[:len(filename)-5])
	}
	_, wellIsIt := validExtensions[ext]
	return wellIsIt
}

// Returns the generated filename for the given original filename
func generatedName(original string) string {
	return original + genExtension
}

func isGeneratedFromGLSL(filename string) bool {
	if strings.HasSuffix(filename, ".gen.go") {
		return isGLSLFile(filename[:len(filename)-7])
	}
	return false
}

// Returns the original filename from the given generated filename
func originalName(generated string) string {
	if !strings.HasSuffix(generated, genExtension) {
		return ""
	}
	return generated[:len(generated)-len(genExtension)]
}

// Returns true if the file 'this' is newer than 'that'.
func isNewer(this, that string) bool {
	dis, err := os.Stat(this)
	if err != nil {
		panic(err)
	}
	dat, err := os.Stat(that)
	if err != nil {
		panic(err)
	}

	return dat.ModTime().Before(dis.ModTime())
}

// makeIdentifier turns filenames into camelcase'd identifiers
func makeIdentifier(s string) string {
	// if strings.HasSuffix(s, ".glsl") {
	// 	s = s[:len(s)-5]
	// }

	var newS string
	capitaliseNext := true
	for _, r := range s {
		if r == '_' || r == '.' || r == '/' {
			capitaliseNext = true
			continue
		}
		if capitaliseNext {
			newS += string(unicode.ToUpper(r))
		} else {
			newS += string(unicode.ToLower(r))
		}
		capitaliseNext = false
	}

	return newS
}
