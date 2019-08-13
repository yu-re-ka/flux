package dependencies

import "net/http"

type Interface interface {
	HTTPClient() *http.Client
	SecretService() SecretService
}
