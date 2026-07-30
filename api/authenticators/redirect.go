package authenticators

import (
	"net/http"
	"net/url"
)

// Add the error query parameter to the redirect URL so the error page
// can show the error.
func RedirectWithError(w http.ResponseWriter, r *http.Request,
	redirect string, err error) {
	redirect_url, err1 := url.Parse(redirect)
	if err1 == nil {
		q := redirect_url.Query()
		q.Set("Error", err.Error())
		redirect_url.RawQuery = q.Encode()
		redirect = redirect_url.String()
	}

	http.Redirect(w, r, redirect, http.StatusTemporaryRedirect)
}
