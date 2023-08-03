package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/ktock/container2wasm/tests/integration/utils"
)

func TestWasmedge(t *testing.T) {
	utils.RunTestRuntimes(t, []utils.TestSpec{
		{
			Name:    "wasmedge-hello",
			Runtime: "wasmedge",
			Inputs: []utils.Input{
				{Image: "alpine:3.17"},
				{Image: "riscv64/alpine:20221110", ConvertOpts: []string{"--target-arch=riscv64"}},
			},
			ImageName: "test2.wasm",
			Prepare: func(t *testing.T, workdir string) {
				assert.NilError(t, exec.Command("wasmedgec", filepath.Join(workdir, "test.wasm"), filepath.Join(workdir, "test2.wasm")).Run())
			},
			Args: utils.StringFlags("--no-stdin", "echo", "-n", "hello"), // NOTE: stdin unsupported on wasmedge as of now
			Want: utils.WantString("hello"),
		},
		// NOTE: stdin unsupported on wasmedge
		// TODO: support it
		{
			Name:    "wasmedge-mapdir",
			Runtime: "wasmedge",
			Inputs: []utils.Input{
				{Image: "alpine:3.17"},
				{Image: "riscv64/alpine:20221110", ConvertOpts: []string{"--target-arch=riscv64"}},
			},
			ImageName: "test2.wasm",
			Prepare: func(t *testing.T, workdir string) {
				assert.NilError(t, exec.Command("wasmedgec", filepath.Join(workdir, "test.wasm"), filepath.Join(workdir, "test2.wasm")).Run())

				mapdirTestDir := filepath.Join(workdir, "wasmedge-mapdirtest/testdir")
				assert.NilError(t, os.MkdirAll(mapdirTestDir, 0755))
				assert.NilError(t, os.WriteFile(filepath.Join(mapdirTestDir, "hi"), []byte("teststring"), 0755))
			},
			Finalize: func(t *testing.T, workdir string) {
				mapdirTestDir := filepath.Join(workdir, "wasmedge-mapdirtest/testdir")
				assert.NilError(t, os.Remove(filepath.Join(mapdirTestDir, "hi")))
				assert.NilError(t, os.Remove(mapdirTestDir))
			},
			RuntimeOpts: func(t *testing.T, workdir string) []string {
				return []string{"--dir=/map/dir:" + filepath.Join(workdir, "wasmedge-mapdirtest/testdir")}
			},
			Args: utils.StringFlags("--no-stdin", "cat", "/map/dir/hi"),
			Want: utils.WantString("teststring"),
		},
	}...)
}