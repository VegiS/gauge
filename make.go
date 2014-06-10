package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	DEPS_DIR  = "deps"
	BUILD_DIR = "tmp"
)

var BUILD_DIR_BIN = filepath.Join(BUILD_DIR, "bin")
var BUILD_DIR_SRC = filepath.Join(BUILD_DIR, "src")
var BUILD_DIR_PKG = filepath.Join(BUILD_DIR, "pkg")

var gaugePackages = []string{"common"}
var gaugeExecutables = []string{"gauge", "gauge-java"}

func isExecMode(mode os.FileMode) bool {
	return (mode & 0111) != 0
}

func mirrorFile(src, dst string) error {
	sfi, err := os.Stat(src)
	if err != nil {
		return err
	}
	if sfi.Mode()&os.ModeType != 0 {
		log.Fatalf("mirrorFile can't deal with non-regular file %s", src)
	}
	dfi, err := os.Stat(dst)
	if err == nil &&
		isExecMode(sfi.Mode()) == isExecMode(dfi.Mode()) &&
		(dfi.Mode()&os.ModeType == 0) &&
		dfi.Size() == sfi.Size() &&
		dfi.ModTime().Unix() == sfi.ModTime().Unix() {
		// Seems to not be modified.
		return nil
	}

	dstDir := filepath.Dir(dst)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}

	df, err := os.Create(dst)
	if err != nil {
		return err
	}
	sf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sf.Close()

	n, err := io.Copy(df, sf)
	if err == nil && n != sfi.Size() {
		err = fmt.Errorf("copied wrong size for %s -> %s: copied %d; want %d", src, dst, n, sfi.Size())
	}
	cerr := df.Close()
	if err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(dst, sfi.Mode())
	}
	if err == nil {
		err = os.Chtimes(dst, sfi.ModTime(), sfi.ModTime())
	}
	return err
}

func mirrorDir(src, dst string) error {
	log.Printf("Copying '%s' -> '%s'\n", src, dst)
	err := filepath.Walk(src, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		suffix, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("Failed to find Rel(%q, %q): %v", src, path, err)
		}
		return mirrorFile(path, filepath.Join(dst, suffix))
	})
	return err
}

func createGoPathForBuild() {
	err := os.MkdirAll(BUILD_DIR_SRC, 0755)
	if err != nil {
		panic(err)
	}

	err = os.MkdirAll(BUILD_DIR_BIN, 0755)
	if err != nil {
		panic(err)
	}

	err = os.MkdirAll(BUILD_DIR_PKG, 0755)
	if err != nil {
		panic(err)
	}
}

func copyDepsToGoPath() {
	err := mirrorDir(DEPS_DIR, BUILD_DIR_SRC)
	if err != nil {
		panic(err)
	}
}

func copyGaugePackagesToGoPath() {
	for _, p := range gaugePackages {
		err := mirrorDir(p, filepath.Join(BUILD_DIR_SRC, p))
		if err != nil {
			panic(err)
		}
	}
	for _, p := range gaugeExecutables {
		err := mirrorDir(p, filepath.Join(BUILD_DIR_SRC, p))
		if err != nil {
			panic(err)
		}
	}
}

func compilePackages() {
	absBuildDir, err := filepath.Abs(BUILD_DIR)
	if err != nil {
		panic(err)
	}
	log.Printf("GOPATH = %s\n", absBuildDir)
	err = os.Setenv("GOPATH", absBuildDir)
	if err != nil {
		panic(err)
	}

	args := []string{"install", "-v"}
	for _, p := range gaugeExecutables {
		args = append(args, p)
	}

	cmd := exec.Command("go", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = BUILD_DIR
	log.Printf("Execute %v\n", cmd.Args)
	err = cmd.Run()
	if err != nil {
		panic(err)
	}
}

func compileJavaClasses() {
	cmd := exec.Command("ant", "jar")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = "gauge-java"
	log.Printf("Execute %v\n", cmd.Args)
	err := cmd.Run()
	if err != nil {
		panic(err)
	}
}

func runTests(packageName string) {
	cmd := exec.Command("go", "test", packageName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = BUILD_DIR
	log.Printf("Execute %v\n", cmd.Args)
	err := cmd.Run()
	if err != nil {
		panic(err)
	}

}

func copyBinaries() {
	err := os.MkdirAll("bin", 0755)
	if err != nil {
		panic(err)
	}

	err = mirrorDir(BUILD_DIR_BIN, "bin")
	if err != nil {
		panic(err)
	}

	absBin, err := filepath.Abs("bin")
	if err != nil {
		panic(err)
	}

	log.Printf("Binaries are available at: %s\n", absBin)
}

func installGaugeFiles() {
	installBin := filepath.Join(*installPrefix, "bin")
	err := mirrorFile(filepath.Join("bin/gauge"), filepath.Join(installBin, "gauge"))
	if err != nil {
		panic(err)
	}
}

func installGaugeJavaFiles() {
	installBin := filepath.Join(*installPrefix, "bin")
	err := mirrorFile(filepath.Join("bin/gauge-java"), installBin)
	if err != nil {
		panic(err)
	}
}

var test = flag.Bool("test", false, "Run the test cases")
var install = flag.Bool("install", false, "Install to the specified prefix")
var installPrefix = flag.String("prefix", "", "Specifies the prefix where files will be installed")

func main() {
	flag.Parse()
	if *test {
		runTests("gauge")
	} else if *install {
		if *installPrefix == "" {
			*installPrefix = "/usr/local"
		}
		installGaugeFiles()
		installGaugeJavaFiles()
	} else {
		createGoPathForBuild()
		copyDepsToGoPath()
		copyGaugePackagesToGoPath()
		compilePackages()
		compileJavaClasses()
		copyBinaries()
	}
}