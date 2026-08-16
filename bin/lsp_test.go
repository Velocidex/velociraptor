package main_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	main "www.velocidex.com/golang/velociraptor/bin"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

const (
	BUFFER_SIZE = 1024 * 1024
)

var (
	lspTestCases = []struct {
		name string
		req  string
	}{
		{
			name: "Initialize call",
			req: `
{
    "jsonrpc": "2.0",
    "id" : 1,
    "method": "initialize",
    "params": {}
}`,
		},
		{
			name: "Add doc",
			req: `
{
    "jsonrpc": "2.0",
    "id" : 2,
    "method": "textDocument/didOpen",
    "params": {
        "textDocument": {
           "uri": "file://XXXX",
           "text": "SELECT * FROM infoXXX()"
        }
    }
}`,
		},
	}
)

type LockedBuffer struct {
	bytes.Buffer
	mu sync.Mutex
}

func (self *LockedBuffer) Write(data []byte) (int, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.Buffer.Write(data)
}

func (self *LockedBuffer) Bytes() []byte {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.Buffer.Bytes()
}

func writeJsonRPC(data string, pipe io.Writer) error {
	msg := fmt.Sprintf("Content-Length: %d\r\n\r\n%s",
		len(data), data)
	_, err := pipe.Write([]byte(msg))
	return err
}

func readFromPipe(ctx context.Context,
	pipe io.Reader, buffer *LockedBuffer) {
	buff := make([]byte, BUFFER_SIZE)
	for !utils.IsCtxDone(ctx) {
		n, err := pipe.Read(buff)
		if err != nil { // EOF on pipe close.
			if n > 0 {
				buffer.Write(buff[:n])
			}
			return
		}

		buffer.Write(buff[:n])
		utils.SleepWithCtx(ctx, 30*time.Millisecond)
	}
}

// End to end test of the lsp server.
func TestLSPServer(t *testing.T) {
	binary, _ := SetupTest(t)

	ctx, cancel := main.Install_sig_handler()
	defer cancel()

	golden := ""
	for idx, tc := range lspTestCases {
		golden += fmt.Sprintf("\nTest Case %d %v:%v\n\n", idx+1,
			tc.name, tc.req)
		resp := CallLSPServer(t, ctx, binary, tc.req)
		golden += indentResp(resp)
	}

	goldie.Assert(t, "TestLSPServer", []byte(golden))
}

var (
	cl_regex = regexp.MustCompile(`^Content-Length: (\d+)\r\n\r\n`)
)

func unframe(in string) (res []string) {
	for {
		m := cl_regex.FindStringSubmatch(in)
		if len(m) == 0 {
			return res
		}

		cl, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			return res
		}

		start := len(m[0])
		res = append(res, in[start:start+int(cl)])
		in = in[start+int(cl):]
	}
}

func indentResp(in string) (res string) {
	for _, item := range unframe(in) {
		dst := bytes.Buffer{}
		err := json.Indent(&dst, []byte(item), "", " ")
		if err == nil {
			res += dst.String() + "\n"
		}
	}
	return res
}

func CallLSPServer(
	t *testing.T,
	ctx context.Context, binary string,
	request_json string) string {
	buffer := &LockedBuffer{}

	command := exec.CommandContext(ctx, binary, "lsp")

	stdin_pipe, err := command.StdinPipe()
	assert.NoError(t, err)
	defer stdin_pipe.Close()

	stdout_pipe, err := command.StdoutPipe()
	assert.NoError(t, err)
	defer stdout_pipe.Close()

	go readFromPipe(ctx, stdout_pipe, buffer)

	stderr_pipe, err := command.StderrPipe()
	assert.NoError(t, err)
	defer stderr_pipe.Close()

	go readFromPipe(ctx, stderr_pipe, buffer)

	err = command.Start()
	assert.NoError(t, err)

	err = writeJsonRPC(request_json, stdin_pipe)
	assert.NoError(t, err)

	vtesting.WaitUntil(5*time.Second, t, func() bool {
		return len(buffer.Bytes()) > 0
	})

	return string(buffer.Bytes())
}
