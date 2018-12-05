package networking

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strings"

	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type _HttpPluginRequest struct {
	Url     string      `vfilter:"required,field=url"`
	Params  vfilter.Any `vfilter:"optional,field=params"`
	Headers vfilter.Any `vfilter:"optional,field=headers"`
	Method  string      `vfilter:"optional,field=method"`
	Chunk   int         `vfilter:"optional,field=chunk_size"`
}

type _HttpPluginResponse struct {
	Url      string
	Content  string
	Response int
}

type _HttpPlugin struct{}

func getHttpClient(arg *_HttpPluginRequest) *http.Client {
	result := &http.Client{}
	// It is a unix domain socket.
	if strings.HasPrefix(arg.Url, "/") {
		components := strings.Split(arg.Url, ":")
		if len(components) == 1 {
			components = append(components, "/")
		}
		result.Transport = &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", components[0])
			},
		}
		arg.Url = "http://unix" + components[1]
	}
	return result
}

func encodeParams(arg *_HttpPluginRequest, scope *vfilter.Scope) *url.Values {
	data := url.Values{}
	if arg.Params != nil {
		for _, member := range scope.GetMembers(arg.Params) {
			value, pres := scope.Associative(arg.Params, member)
			if pres {
				slice := reflect.ValueOf(value)
				if slice.Type().Kind() == reflect.Slice {
					for i := 0; i < slice.Len(); i++ {
						value := slice.Index(i).Interface()
						item, ok := value.(string)
						if ok {
							data.Add(member, item)
							continue
						}
					}
				}
				switch value.(type) {
				case vfilter.Null, *vfilter.Null:
					continue
				default:
					data.Add(member, fmt.Sprintf("%v", value))
				}
			}
		}
	}
	return &data
}

func (self *_HttpPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	arg := &_HttpPluginRequest{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		goto error
	}

	if arg.Chunk == 0 {
		arg.Chunk = 4 * 1024 * 1024
	}

	if arg.Method == "" {
		arg.Method = "GET"
	}

	go func() {
		defer close(output_chan)
		params := encodeParams(arg, scope)
		client := getHttpClient(arg)
		req, err := http.NewRequest(
			arg.Method, arg.Url,
			strings.NewReader(params.Encode()))
		if err != nil {
			scope.Log("%s: %s", self.Name(), err.Error())
			return
		}

		http_resp, err := client.Do(req)
		if err != nil {
			output_chan <- &_HttpPluginResponse{
				Url:      arg.Url,
				Response: 500,
				Content:  err.Error(),
			}
			return
		}
		defer http_resp.Body.Close()

		response := &_HttpPluginResponse{
			Url:      arg.Url,
			Response: http_resp.StatusCode,
		}

		buf := make([]byte, arg.Chunk)
		for {
			n, err := http_resp.Body.Read(buf)
			if err != nil && err == io.EOF {
				response.Content = string(buf[:n])
				output_chan <- response
				return
			}

			response.Content = string(buf[:n])
			output_chan <- response
		}
	}()

	return output_chan

error:
	scope.Log("%s: %s", self.Name(), err.Error())
	close(output_chan)
	return output_chan
}

func (self _HttpPlugin) Name() string {
	return "http_client"
}

func (self _HttpPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    self.Name(),
		Doc:     "Make a http request.",
		RowType: type_map.AddType(scope, &_HttpPluginResponse{}),
		ArgType: type_map.AddType(scope, &_HttpPluginRequest{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_HttpPlugin{})
}
