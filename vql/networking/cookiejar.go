package networking

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"

	"github.com/Velocidex/ordereddict"
	"golang.org/x/net/publicsuffix"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
)

type DictBasedCookieJar struct {
	dict *ordereddict.Dict
	*cookiejar.Jar
}

// NOTE: The logic of which cookie to use in which site is actually
// fairly tricky so we leave it to the official cookie jar - we just
// record the SetCookie calls that each site places and then when the
// Jar is constructed we replay those calls into it. This allows the
// official Jar to implement the cookie logic, and leave the dict to
// just take care of the storage.
func NewDictJar(dict *ordereddict.Dict) http.CookieJar {
	if dict == nil {
		dict = ordereddict.NewDict()
	}

	// Initialize the Jar from the dict we were given.
	jar, err := cookiejar.New(&cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	})
	if err != nil {
		return nil
	}

	self := &DictBasedCookieJar{
		dict: dict,
		Jar:  jar,
	}

	for _, i := range dict.Items() {
		url, err := url.Parse(i.Key)
		if err != nil {
			continue
		}

		if !utils.IsNil(i.Value) {
			cookies, err := member_to_cookies(i.Value)
			if err == nil {
				self.Jar.SetCookies(url, cookies)
			}
		}
	}

	return self
}

// Intercept calls to SetCookies and copy the cookies to the dict.
func (self *DictBasedCookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	key := u.String()
	self.dict.Set(key, cookies_to_dicts(cookies))

	self.Jar.SetCookies(u, cookies)
}

func member_to_cookies(in interface{}) ([]*http.Cookie, error) {
	serialized, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}

	result := []*http.Cookie{}
	err = json.Unmarshal(serialized, &result)
	return result, err
}

func cookies_to_dicts(cookies []*http.Cookie) []*ordereddict.Dict {
	result := []*ordereddict.Dict{}
	for _, c := range cookies {
		serialized, err := json.Marshal(c)
		if err != nil {
			continue
		}

		item, err := utils.ParseJsonToObject(serialized)
		if err != nil {
			continue
		}

		result = append(result, item)
	}
	return result
}
