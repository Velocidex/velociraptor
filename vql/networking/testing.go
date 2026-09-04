package networking

var (
	HTTPClientMock HTTPClient
)

func MockHTTPClient(mock HTTPClient) func() {
	HTTPClientMock = mock
	return func() {
		HTTPClientMock = nil
	}
}
